package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ChangeRecord represents a database change that needs to be synced
type ChangeRecord struct {
	ID        int64          `json:"id"`
	TableName string         `json:"table_name"`
	Operation string         `json:"operation"` // INSERT, UPDATE, DELETE
	RecordID  string         `json:"record_id"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
	NodeID    string         `json:"node_id"` // Deprecated: Use Hotkey instead
	Hotkey    string         `json:"hotkey"`  // The hotkey address of the node that created the change
	Synced    bool           `json:"synced"`
}

// SyncManager handles database synchronization between nodes
type SyncManager struct {
	db     Database
	hotkey string // Changed from nodeID to hotkey for clarity
}

// NewSyncManager creates a new sync manager
func NewSyncManager(db Database, hotkey string) (*SyncManager, error) {
	if err := initSyncTables(context.Background(), db); err != nil {
		return nil, err
	}

	return &SyncManager{
		db:     db,
		hotkey: hotkey,
	}, nil
}

// initSyncTables creates necessary tables for tracking changes
func initSyncTables(ctx context.Context, db Database) error {
	return db.WithTx(ctx, func(tx pgx.Tx) error {
		// Create table for tracking changes
		_, err := tx.Exec(ctx, `
			CREATE TABLE IF NOT EXISTS sync_changes (
				id BIGSERIAL PRIMARY KEY,
				table_name TEXT NOT NULL,
				operation TEXT NOT NULL,
				record_id TEXT NOT NULL,
				data JSONB,
				timestamp TIMESTAMPTZ DEFAULT NOW(),
				hotkey TEXT NOT NULL, -- Changed from node_id to hotkey
				synced BOOLEAN DEFAULT FALSE
			)
		`)
		if err != nil {
			return err
		}

		// Create index for faster syncing
		_, err = tx.Exec(ctx, `
			CREATE INDEX IF NOT EXISTS idx_sync_changes_unsynced 
			ON sync_changes (synced, timestamp) 
			WHERE NOT synced
		`)
		return err
	})
}

// RecordChange records a database change that needs to be synced
func (sm *SyncManager) RecordChange(ctx context.Context, tableName, operation, recordID string, data map[string]any) error {
	return sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO sync_changes (table_name, operation, record_id, data, hotkey) 
			 VALUES ($1, $2, $3, $4, $5)`,
			tableName,
			operation,
			recordID,
			data,
			sm.hotkey,
		)
		return err
	})
}

// GetUnsynced retrieves unsynced changes since a given timestamp
func (sm *SyncManager) GetUnsynced(ctx context.Context, since time.Time) ([]ChangeRecord, error) {
	var changes []ChangeRecord

	err := sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, table_name, operation, record_id, data, timestamp, hotkey, synced 
			 FROM sync_changes 
			 WHERE NOT synced AND timestamp > $1
			 ORDER BY timestamp ASC`,
			since,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var change ChangeRecord
			var dataJSON []byte
			err := rows.Scan(
				&change.ID,
				&change.TableName,
				&change.Operation,
				&change.RecordID,
				&dataJSON,
				&change.Timestamp,
				&change.NodeID, // Keep as NodeID in the struct for backward compatibility
				&change.Synced,
			)
			if err != nil {
				return err
			}

			if err := json.Unmarshal(dataJSON, &change.Data); err != nil {
				return err
			}

			changes = append(changes, change)
		}

		return rows.Err()
	})

	return changes, err
}

// MarkSynced marks changes as synced
func (sm *SyncManager) MarkSynced(ctx context.Context, ids []int64) error {
	return sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE sync_changes SET synced = true WHERE id = ANY($1)`,
			ids,
		)
		return err
	})
}

// ApplyChanges applies changes from another node
func (sm *SyncManager) ApplyChanges(ctx context.Context, changes []ChangeRecord) error {
	return sm.db.WithTx(ctx, func(tx pgx.Tx) error {
		for _, change := range changes {
			switch change.Operation {
			case "INSERT", "UPDATE":
				// Convert data to key-value pairs for SQL
				cols := make([]string, 0, len(change.Data))
				vals := make([]any, 0, len(change.Data))
				i := 1
				for k, v := range change.Data {
					cols = append(cols, k)
					vals = append(vals, v)
					i++
				}

				query := buildUpsertQuery(change.TableName, cols)
				_, err := tx.Exec(ctx, query, vals...)
				if err != nil {
					return err
				}

			case "DELETE":
				_, err := tx.Exec(ctx,
					`DELETE FROM `+change.TableName+` WHERE id = $1`,
					change.RecordID,
				)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Helper function to build UPSERT query
func buildUpsertQuery(table string, columns []string) string {
	colList := strings.Join(columns, ", ")
	placeholders := make([]string, len(columns))
	updates := make([]string, len(columns))

	for i := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		updates[i] = fmt.Sprintf("%s = EXCLUDED.%s", columns[i], columns[i])
	}

	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (id) DO UPDATE SET %s",
		table,
		colList,
		strings.Join(placeholders, ", "),
		strings.Join(updates, ", "),
	)
}
