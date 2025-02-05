package substrate

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"
	"time"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
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

// SetWeightsConfig holds configuration for setting weights
type SetWeightsConfig struct {
	MaxAttempts int
	RetryDelay  time.Duration
}

// DefaultSetWeightsConfig returns default configuration for setting weights
func DefaultSetWeightsConfig() SetWeightsConfig {
	return SetWeightsConfig{
		MaxAttempts: 3,
		RetryDelay:  time.Second * 30,
	}
}

// SetWeights sets the weights for a validator in a subnet
func (c *Client) SetWeights(ctx context.Context, netuid types.U16, weights map[types.AccountID]types.U16, keypair signature.KeyringPair, config SetWeightsConfig) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("client not connected")
	}

	// Create call for set_weights
	call, err := types.NewCall(
		c.metadata,
		"SubtensorModule.set_weights",
		netuid,
		weights,
	)
	if err != nil {
		return fmt.Errorf("creating set_weights call: %w", err)
	}

	// Create signed extrinsic
	ext := types.NewExtrinsic(call)
	era := types.ExtrinsicEra{IsMortalEra: false}

	genesisHash, err := c.api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		return fmt.Errorf("getting genesis hash: %w", err)
	}

	rv, err := c.api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		return fmt.Errorf("getting runtime version: %w", err)
	}

	// Sign the extrinsic
	err = ext.Sign(keypair, types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                era,
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(0), // Nonce will be set by the node
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	})
	if err != nil {
		return fmt.Errorf("signing extrinsic: %w", err)
	}

	// Submit with retries
	var lastErr error
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Check if weights can be set
		canSet, err := c.CanSetWeights(ctx, netuid, keypair.PublicKey)
		if err != nil {
			lastErr = fmt.Errorf("checking if weights can be set: %w", err)
			time.Sleep(config.RetryDelay)
			continue
		}
		if !canSet {
			lastErr = fmt.Errorf("weights cannot be set yet")
			time.Sleep(config.RetryDelay)
			continue
		}

		// Submit extrinsic
		_, err = c.api.RPC.Author.SubmitExtrinsic(ext)
		if err != nil {
			lastErr = fmt.Errorf("submitting extrinsic: %w", err)
			time.Sleep(config.RetryDelay)
			continue
		}

		// Success
		return nil
	}

	return fmt.Errorf("failed to set weights after %d attempts: %w", config.MaxAttempts, lastErr)
}

// CanSetWeights checks if a validator can set weights
func (c *Client) CanSetWeights(ctx context.Context, netuid types.U16, validatorID []byte) (bool, error) {
	// Get blocks since last update
	lastUpdate, err := c.getLastUpdate(netuid, validatorID)
	if err != nil {
		return false, fmt.Errorf("getting last update: %w", err)
	}

	header, err := c.api.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return false, fmt.Errorf("getting current block: %w", err)
	}

	blocksSinceUpdate := uint64(header.Number) - uint64(lastUpdate)

	// Get minimum interval
	minInterval, err := c.getWeightsSetRateLimit(netuid)
	if err != nil {
		return false, fmt.Errorf("getting weights set rate limit: %w", err)
	}

	return blocksSinceUpdate >= uint64(minInterval), nil
}

// getLastUpdate gets the last block number when a validator updated weights
func (c *Client) getLastUpdate(netuid types.U16, validatorID []byte) (types.BlockNumber, error) {
	key, err := types.CreateStorageKey(c.metadata, "SubtensorModule", "LastUpdate")
	if err != nil {
		return 0, fmt.Errorf("creating storage key: %w", err)
	}

	// Append netuid and validatorID to key
	key = append(key, uint16ToBytes(uint16(netuid))...)
	key = append(key, validatorID...)

	var lastUpdate types.BlockNumber
	ok, err := c.api.RPC.State.GetStorageLatest(key, &lastUpdate)
	if err != nil {
		return 0, fmt.Errorf("querying last update: %w", err)
	}
	if !ok {
		return 0, nil // No previous update
	}

	return lastUpdate, nil
}

// getWeightsSetRateLimit gets the minimum interval between weight updates
func (c *Client) getWeightsSetRateLimit(netuid types.U16) (types.U16, error) {
	key, err := types.CreateStorageKey(c.metadata, "SubtensorModule", "WeightsSetRateLimit")
	if err != nil {
		return 0, fmt.Errorf("creating storage key: %w", err)
	}

	// Append netuid to key
	key = append(key, uint16ToBytes(uint16(netuid))...)

	var rateLimit types.U16
	ok, err := c.api.RPC.State.GetStorageLatest(key, &rateLimit)
	if err != nil {
		return 0, fmt.Errorf("querying rate limit: %w", err)
	}
	if !ok {
		return 0, fmt.Errorf("rate limit not found for netuid %d", netuid)
	}

	return rateLimit, nil
}

// uint16ToBytes converts a uint16 to a little-endian byte slice
func uint16ToBytes(n uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, n)
	return buf
}

// ServeAxonConfig holds configuration for serving an axon
type ServeAxonConfig struct {
	MaxAttempts int
	RetryDelay  time.Duration
}

// DefaultServeAxonConfig returns default configuration for serving an axon
func DefaultServeAxonConfig() ServeAxonConfig {
	return ServeAxonConfig{
		MaxAttempts: 3,
		RetryDelay:  time.Second * 10,
	}
}

// ServeAxon registers a validator's axon endpoint on the network
func (c *Client) ServeAxon(ctx context.Context, netuid types.U16, ip string, port uint16, keypair signature.KeyringPair, version string, config ServeAxonConfig) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("client not connected")
	}

	// Create call for serve_axon
	call, err := types.NewCall(
		c.metadata,
		"SubtensorModule.serve_axon",
		netuid,
		ip,
		types.NewU16(port),
		version,
	)
	if err != nil {
		return fmt.Errorf("creating serve_axon call: %w", err)
	}

	// Create signed extrinsic
	ext := types.NewExtrinsic(call)
	era := types.ExtrinsicEra{IsMortalEra: false}

	genesisHash, err := c.api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		return fmt.Errorf("getting genesis hash: %w", err)
	}

	rv, err := c.api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		return fmt.Errorf("getting runtime version: %w", err)
	}

	// Sign the extrinsic
	err = ext.Sign(keypair, types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                era,
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(0),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	})
	if err != nil {
		return fmt.Errorf("signing extrinsic: %w", err)
	}

	// Submit with retries
	var lastErr error
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Submit extrinsic
		_, err = c.api.RPC.Author.SubmitExtrinsic(ext)
		if err != nil {
			lastErr = fmt.Errorf("submitting extrinsic: %w", err)
			time.Sleep(config.RetryDelay)
			continue
		}

		// Success
		return nil
	}

	return fmt.Errorf("failed to serve axon after %d attempts: %w", config.MaxAttempts, lastErr)
}

// GetStake returns the stake amount for a validator
func (c *Client) GetStake(ctx context.Context, hotkey, coldkey types.AccountID) (types.U128, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return types.U128{}, fmt.Errorf("client not connected")
	}

	// Create storage key for stake
	key, err := types.CreateStorageKey(c.metadata, "SubtensorModule", "Stake")
	if err != nil {
		return types.U128{}, fmt.Errorf("creating storage key: %w", err)
	}

	// Append hotkey and coldkey to key
	key = append(key, hotkey[:]...)
	key = append(key, coldkey[:]...)

	var stake types.U128
	ok, err := c.api.RPC.State.GetStorageLatest(key, &stake)
	if err != nil {
		return types.U128{}, fmt.Errorf("querying stake: %w", err)
	}
	if !ok {
		return types.NewU128(*big.NewInt(0)), nil
	}

	return stake, nil
}

// StakeConfig holds configuration for staking operations
type StakeConfig struct {
	MaxAttempts int
	RetryDelay  time.Duration
}

// DefaultStakeConfig returns default configuration for staking operations
func DefaultStakeConfig() StakeConfig {
	return StakeConfig{
		MaxAttempts: 3,
		RetryDelay:  time.Second * 10,
	}
}

// executeStakeOperation handles common stake operation logic
func (c *Client) executeStakeOperation(
	ctx context.Context,
	operation string,
	amount types.U128,
	hotkey types.AccountID,
	keypair signature.KeyringPair,
	config StakeConfig,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("client not connected")
	}

	// Create extrinsic builder
	builder := NewExtrinsicBuilder(c.api, c.metadata)

	var lastErr error
	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Build the extrinsic
		module := "SubtensorModule"
		function := "add_stake"
		if operation == "remove" {
			function = "remove_stake"
		}

		builder, err := builder.WithCall(module, function, hotkey, amount)
		if err != nil {
			return fmt.Errorf("creating call: %w", err)
		}

		ext, err := builder.Build(keypair)
		if err != nil {
			return fmt.Errorf("building extrinsic: %w", err)
		}

		// Submit extrinsic
		sub, err := c.api.RPC.Author.SubmitAndWatchExtrinsic(ext)
		if err != nil {
			lastErr = fmt.Errorf("submitting extrinsic: %w", err)
			time.Sleep(config.RetryDelay)
			continue
		}

		defer sub.Unsubscribe()

		// Wait for inclusion or error
		select {
		case <-ctx.Done():
			return ctx.Err()
		case status := <-sub.Chan():
			if status.IsInBlock {
				return nil
			}
			if status.IsDropped || status.IsInvalid {
				lastErr = fmt.Errorf("extrinsic dropped/invalid")
				time.Sleep(config.RetryDelay)
				continue
			}
		}
	}

	return fmt.Errorf("failed to execute stake operation after %d attempts: %w", config.MaxAttempts, lastErr)
}

// AddStake adds stake to a validator
func (c *Client) AddStake(ctx context.Context, amount types.U128, hotkey types.AccountID, keypair signature.KeyringPair, config StakeConfig) error {
	return c.executeStakeOperation(ctx, "add", amount, hotkey, keypair, config)
}

// RemoveStake removes stake from a validator
func (c *Client) RemoveStake(ctx context.Context, amount types.U128, hotkey types.AccountID, keypair signature.KeyringPair, config StakeConfig) error {
	return c.executeStakeOperation(ctx, "remove", amount, hotkey, keypair, config)
}

// QueryAxonInfo returns the IP, port and version for a validator
func (c *Client) QueryAxonInfo(ctx context.Context, netuid types.U16, hotkey types.AccountID) (string, uint16, string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return "", 0, "", fmt.Errorf("client not connected")
	}

	// Create storage key for axon info
	meta := c.GetMetadata()
	key, err := types.CreateStorageKey(meta, "SubtensorModule", "Axons", netuidToBytes(netuid), hotkey[:])
	if err != nil {
		return "", 0, "", fmt.Errorf("creating storage key: %w", err)
	}

	// Query storage
	var axonInfo struct {
		IP      string
		Port    types.U16
		Version string
	}
	ok, err := c.api.RPC.State.GetStorageLatest(key, &axonInfo)
	if err != nil {
		return "", 0, "", fmt.Errorf("querying axon info: %w", err)
	}
	if !ok {
		return "", 0, "", fmt.Errorf("axon info not found")
	}

	return axonInfo.IP, uint16(axonInfo.Port), axonInfo.Version, nil
}
