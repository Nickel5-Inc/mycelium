package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// PubSubConfig holds configuration for the pub/sub system
type PubSubConfig struct {
	// Maximum age of messages in the stream
	MaxAge time.Duration
	// Storage type (file or memory)
	StorageType nats.StorageType
}

// MinerUpdate represents an update to be sent to miners
type MinerUpdate struct {
	ShardID string
	Type    string
	Data    []byte
}

// PubSubManager handles pub/sub communication with miners
type PubSubManager struct {
	mu sync.RWMutex
	// NATS connection
	nc *nats.Conn
	// NATS JetStream context
	js nats.JetStreamContext
	// Subjects by shard
	subjects map[string]string
	// Configuration
	config PubSubConfig
}

// NewPubSubManager creates a new pub/sub manager for miner communication
func NewPubSubManager(url string, config PubSubConfig) (*PubSubManager, error) {
	// Connect to NATS
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return &PubSubManager{
		nc:       nc,
		js:       js,
		subjects: make(map[string]string),
		config:   config,
	}, nil
}

// CreateShardStream creates a new stream for a shard
func (pm *PubSubManager) CreateShardStream(shardID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	subject := fmt.Sprintf("shard.%s", shardID)

	// Create stream
	_, err := pm.js.AddStream(&nats.StreamConfig{
		Name:     fmt.Sprintf("SHARD_%s", shardID),
		Subjects: []string{subject},
		MaxAge:   pm.config.MaxAge,
		Storage:  pm.config.StorageType,
	})
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	pm.subjects[shardID] = subject
	return nil
}

// PublishUpdate publishes an update to miners for a specific shard
func (pm *PubSubManager) PublishUpdate(ctx context.Context, update MinerUpdate) error {
	pm.mu.RLock()
	subject, exists := pm.subjects[update.ShardID]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no stream exists for shard %s", update.ShardID)
	}

	// Publish with headers
	msg := &nats.Msg{
		Subject: subject,
		Data:    update.Data,
		Header: nats.Header{
			"Type":     []string{update.Type},
			"Shard-ID": []string{update.ShardID},
		},
	}

	// Publish with context
	_, err := pm.js.PublishMsg(msg)
	if err != nil {
		return fmt.Errorf("failed to publish update: %w", err)
	}

	return nil
}

// SubscribeToShard subscribes a miner to updates for a specific shard
func (pm *PubSubManager) SubscribeToShard(shardID string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	pm.mu.RLock()
	subject, exists := pm.subjects[shardID]
	pm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no stream exists for shard %s", shardID)
	}

	// Create durable consumer
	sub, err := pm.js.Subscribe(subject, handler, nats.Durable(fmt.Sprintf("miner-%s", shardID)))
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	return sub, nil
}

// UnsubscribeFromShard unsubscribes a miner from shard updates
func (pm *PubSubManager) UnsubscribeFromShard(sub *nats.Subscription) error {
	return sub.Unsubscribe()
}

// Close closes the NATS connection
func (pm *PubSubManager) Close() error {
	pm.nc.Close()
	return nil
}
