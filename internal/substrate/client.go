package substrate

import (
	"context"
	"fmt"
	"sync"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// Config holds the configuration for the Substrate client
type Config struct {
	// WebSocket endpoint URL (e.g. "ws://127.0.0.1:9944")
	Endpoint string

	// SS58 format for address encoding (42 for Bittensor)
	SS58Format uint8

	// Optional timeout for operations
	Timeout types.BlockNumber
}

// Client manages the connection to a Substrate node
type Client struct {
	mu sync.RWMutex

	// Core API client
	api *gsrpc.SubstrateAPI

	// Cached chain metadata
	metadata *types.Metadata

	// Configuration
	config Config

	// Connection state
	connected bool
}

// NewClient creates a new Substrate client
func NewClient(config Config) (*Client, error) {
	api, err := gsrpc.NewSubstrateAPI(config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create substrate API: %w", err)
	}

	client := &Client{
		api:       api,
		config:    config,
		connected: true, // Set initial connection state
	}

	// Initialize metadata
	if err := client.updateMetadata(); err != nil {
		return nil, fmt.Errorf("failed to initialize metadata: %w", err)
	}

	return client, nil
}

// updateMetadata fetches and caches the latest chain metadata
func (c *Client) updateMetadata() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	meta, err := c.api.RPC.State.GetMetadataLatest()
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	c.metadata = meta
	return nil
}

// GetMetadata returns the cached chain metadata
func (c *Client) GetMetadata() *types.Metadata {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metadata
}

// Close closes the connection to the Substrate node
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	return nil
}

// QueryStorage performs a storage query for a given module and function
func (c *Client) QueryStorage(ctx context.Context, module, function string, key []byte) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("client not connected")
	}

	// Create storage key
	meta := c.GetMetadata()
	storageKey, err := types.CreateStorageKey(meta, module, function, key)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage key: %w", err)
	}

	// Query latest state
	var raw types.StorageDataRaw
	exists, err := c.api.RPC.State.GetStorageLatest(storageKey, &raw)
	if err != nil {
		return nil, fmt.Errorf("failed to query storage: %w", err)
	}
	if !exists {
		return nil, nil
	}

	return raw, nil
}

// SubscribeStorage subscribes to storage changes for given keys
func (c *Client) SubscribeStorage(ctx context.Context, keys [][]byte) (<-chan types.StorageChangeSet, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, fmt.Errorf("client not connected")
	}

	// Convert raw keys to storage keys
	storageKeys := make([]types.StorageKey, len(keys))
	for i, key := range keys {
		storageKeys[i] = types.StorageKey(key)
	}

	sub, err := c.api.RPC.State.SubscribeStorageRaw(storageKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to storage: %w", err)
	}

	// Create channel for changes
	ch := make(chan types.StorageChangeSet)

	// Handle subscription in background
	go func() {
		defer close(ch)
		defer sub.Unsubscribe()

		for {
			select {
			case <-ctx.Done():
				return
			case set := <-sub.Chan():
				ch <- set
			}
		}
	}()

	return ch, nil
}
