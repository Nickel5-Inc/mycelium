package database

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// ShardManager handles data sharding and replication
type ShardManager struct {
	db       Database
	nodeID   string
	nodeType string // 'validator' or 'miner'
	mu       sync.RWMutex
	shards   map[string]*ShardInfo
	replicas map[string][]string // shardID -> []nodeID
}

// ShardInfo contains information about a shard
type ShardInfo struct {
	ShardID       string
	StartKey      string
	EndKey        string
	IsPrimary     bool
	ReplicaCount  int
	LastSync      time.Time
	Status        string
	Metadata      map[string]string
	PartitionName string
	NodeType      string
}

// NewShardManager creates a new shard manager
func NewShardManager(db Database, nodeID string, nodeType string) *ShardManager {
	if nodeType != "validator" && nodeType != "miner" {
		panic("invalid node type: must be 'validator' or 'miner'")
	}
	return &ShardManager{
		db:       db,
		nodeID:   nodeID,
		nodeType: nodeType,
		shards:   make(map[string]*ShardInfo),
		replicas: make(map[string][]string),
	}
}

// InitializeShard initializes a new shard
func (sm *ShardManager) InitializeShard(ctx context.Context, info ShardInfo) error {
	if sm.nodeType != "validator" {
		return fmt.Errorf("only validators can initialize shards")
	}

	return sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		// Generate partition name
		info.PartitionName = fmt.Sprintf("data_shard_%s", info.ShardID)
		info.NodeType = sm.nodeType

		// Create partition for the shard
		_, err := tx.Exec(ctx, `SELECT create_shard_partition($1, $2)`,
			info.ShardID, info.PartitionName)
		if err != nil {
			return fmt.Errorf("failed to create shard partition: %w", err)
		}

		// Create shard allocation record
		_, err = tx.Exec(ctx, `
			INSERT INTO shard_allocations 
			(shard_id, node_id, is_primary, replica_count, status, metadata, partition_name)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`,
			info.ShardID,
			sm.nodeID,
			info.IsPrimary,
			info.ReplicaCount,
			info.Status,
			info.Metadata,
			info.PartitionName,
		)
		if err != nil {
			return fmt.Errorf("failed to create shard allocation: %w", err)
		}

		sm.mu.Lock()
		sm.shards[info.ShardID] = &info
		sm.mu.Unlock()

		return nil
	})
}

// AssignReplica assigns a replica of a shard to a node
func (sm *ShardManager) AssignReplica(ctx context.Context, shardID, nodeID string, nodeType string) error {
	if sm.nodeType != "validator" {
		return fmt.Errorf("only validators can assign replicas")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return fmt.Errorf("shard %s not found", shardID)
	}

	return sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		// Setup foreign table for the replica with appropriate permissions
		serverName := fmt.Sprintf("replica_server_%s_%s", shardID, nodeID)
		_, err := tx.Exec(ctx, `SELECT setup_foreign_shard($1, $2, $3, $4, $5)`,
			shardID, serverName, "public", shard.PartitionName, nodeType)
		if err != nil {
			return fmt.Errorf("failed to setup foreign shard: %w", err)
		}

		// Create sync status record
		_, err = tx.Exec(ctx, `
			INSERT INTO shard_sync_status 
			(shard_id, source_node, target_node, sync_started, status)
			VALUES ($1, $2, $3, $4, $5)
		`,
			shardID,
			sm.nodeID,
			nodeID,
			time.Now(),
			"pending",
		)
		if err != nil {
			return fmt.Errorf("failed to create sync status: %w", err)
		}

		// Update replica count
		shard.ReplicaCount++
		_, err = tx.Exec(ctx, `
			UPDATE shard_allocations 
			SET replica_count = $1 
			WHERE shard_id = $2
		`,
			shard.ReplicaCount,
			shardID,
		)
		if err != nil {
			return fmt.Errorf("failed to update replica count: %w", err)
		}

		sm.replicas[shardID] = append(sm.replicas[shardID], nodeID)
		return nil
	})
}

// SyncShard synchronizes a shard with its replicas
func (sm *ShardManager) SyncShard(ctx context.Context, shardID string) error {
	sm.mu.RLock()
	shard, exists := sm.shards[shardID]
	replicas := sm.replicas[shardID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("shard %s not found", shardID)
	}

	return sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		// Get unsynced changes
		rows, err := tx.Query(ctx, `
			SELECT id, table_name, operation, record_id, data 
			FROM sync_changes 
			WHERE shard_id = $1 AND NOT is_synced
			ORDER BY timestamp ASC
		`, shardID)
		if err != nil {
			return fmt.Errorf("failed to get unsynced changes: %w", err)
		}
		defer rows.Close()

		var changes []struct {
			ID        int64
			Table     string
			Operation string
			RecordID  string
			Data      map[string]interface{}
		}

		for rows.Next() {
			var change struct {
				ID        int64
				Table     string
				Operation string
				RecordID  string
				Data      map[string]interface{}
			}
			if err := rows.Scan(&change.ID, &change.Table, &change.Operation,
				&change.RecordID, &change.Data); err != nil {
				return fmt.Errorf("failed to scan change: %w", err)
			}
			changes = append(changes, change)
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("error iterating changes: %w", err)
		}

		// Update sync status for each replica
		for _, nodeID := range replicas {
			_, err := tx.Exec(ctx, `
				UPDATE shard_sync_status 
				SET sync_completed = $1, status = $2
				WHERE shard_id = $3 AND target_node = $4
			`,
				time.Now(),
				"completed",
				shardID,
				nodeID,
			)
			if err != nil {
				return fmt.Errorf("failed to update sync status: %w", err)
			}
		}

		// Mark changes as synced
		_, err = tx.Exec(ctx, `
			UPDATE sync_changes 
			SET is_synced = true 
			WHERE shard_id = $1 AND NOT is_synced
		`, shardID)
		if err != nil {
			return fmt.Errorf("failed to mark changes as synced: %w", err)
		}

		// Update shard last sync time
		shard.LastSync = time.Now()
		_, err = tx.Exec(ctx, `
			UPDATE shard_allocations 
			SET last_sync = $1 
			WHERE shard_id = $2
		`,
			shard.LastSync,
			shardID,
		)
		if err != nil {
			return fmt.Errorf("failed to update last sync time: %w", err)
		}

		return nil
	})
}

// GetShardInfo returns information about a shard
func (sm *ShardManager) GetShardInfo(shardID string) (*ShardInfo, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shard, exists := sm.shards[shardID]
	if !exists {
		return nil, fmt.Errorf("shard %s not found", shardID)
	}
	return shard, nil
}

// GetShardReplicas returns the list of replica nodes for a shard
func (sm *ShardManager) GetShardReplicas(shardID string) ([]string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	replicas, exists := sm.replicas[shardID]
	if !exists {
		return nil, fmt.Errorf("shard %s not found", shardID)
	}
	return replicas, nil
}

// LoadShards loads all shards assigned to this node
func (sm *ShardManager) LoadShards(ctx context.Context) error {
	return sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT 
				shard_id, 
				is_primary, 
				replica_count, 
				last_sync, 
				status, 
				metadata
			FROM shard_allocations 
			WHERE node_id = $1
		`, sm.nodeID)
		if err != nil {
			return fmt.Errorf("failed to query shards: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var info ShardInfo
			if err := rows.Scan(
				&info.ShardID,
				&info.IsPrimary,
				&info.ReplicaCount,
				&info.LastSync,
				&info.Status,
				&info.Metadata,
			); err != nil {
				return fmt.Errorf("failed to scan shard info: %w", err)
			}

			sm.mu.Lock()
			sm.shards[info.ShardID] = &info
			sm.mu.Unlock()
		}

		return rows.Err()
	})
}
