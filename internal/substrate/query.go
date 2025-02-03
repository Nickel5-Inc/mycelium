package substrate

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// Storage prefixes and keys
const (
	// Module prefix for subtensor storage
	SubtensorPrefix = "SubtensorModule"
	// Storage key for weights
	WeightsKey = "Weights"
	// Storage key for subnets
	SubnetworkNKey = "SubnetworkN"
	// Storage key for total networks
	TotalNetworksKey = "TotalNetworks"
	// Storage key for neuron count
	TotalIssuanceKey = "TotalIssuance"
	// Storage key for neurons
	NeuronsKey = "Neurons"
)

// Weight represents a validator's weight assignment to another validator
type Weight struct {
	// Source validator's hotkey
	Source types.AccountID
	// Target validator's hotkey
	Target types.AccountID
	// Weight value (normalized to u16::MAX)
	Value types.U16
	// Subnet ID
	NetUID types.U16
}

// netuidToBytes converts a netuid to its byte representation
func netuidToBytes(netuid types.U16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(netuid))
	return buf
}

// GetValidatorWeight queries the weight set by one validator for another in a specific subnet
func (c *Client) GetValidatorWeight(netuid types.U16, source, target types.AccountID) (types.U16, error) {
	if !c.connected {
		return 0, fmt.Errorf("client not connected")
	}

	// Generate storage key for weights
	// The weights map is double-map: (netuid, source) -> target -> weight
	key, err := types.CreateStorageKey(c.metadata, SubtensorPrefix, WeightsKey, netuidToBytes(netuid), source[:])
	if err != nil {
		return 0, fmt.Errorf("creating storage key: %w", err)
	}

	var weight types.U16
	ok, err := c.api.RPC.State.GetStorageLatest(key, &weight)
	if err != nil {
		return 0, fmt.Errorf("querying storage: %w", err)
	}
	if !ok {
		return 0, nil // No weight set
	}

	return weight, nil
}

// GetAllValidatorWeights queries all weights set by a validator in a specific subnet
func (c *Client) GetAllValidatorWeights(netuid types.U16, source types.AccountID) ([]Weight, error) {
	if !c.connected {
		return nil, fmt.Errorf("client not connected")
	}

	// Check if the subnet exists by querying the total networks
	totalNetworksKey, err := types.CreateStorageKey(c.metadata, SubtensorPrefix, TotalNetworksKey)
	if err != nil {
		return nil, fmt.Errorf("creating total networks key: %w", err)
	}

	var totalNetworks types.U16
	ok, err := c.api.RPC.State.GetStorageLatest(totalNetworksKey, &totalNetworks)
	if err != nil {
		return nil, fmt.Errorf("querying total networks: %w", err)
	}
	if !ok || netuid >= totalNetworks {
		return nil, fmt.Errorf("subnet %d not found", netuid)
	}

	// Check if the subnet is active
	subnetKey, err := types.CreateStorageKey(c.metadata, SubtensorPrefix, SubnetworkNKey, netuidToBytes(netuid))
	if err != nil {
		return nil, fmt.Errorf("creating subnet key: %w", err)
	}

	var subnetExists bool
	ok, err = c.api.RPC.State.GetStorageLatest(subnetKey, &subnetExists)
	if err != nil {
		return nil, fmt.Errorf("querying subnet: %w", err)
	}
	if !ok || !subnetExists {
		return nil, fmt.Errorf("subnet %d not found", netuid)
	}

	// Generate storage prefix for all weights from this validator
	prefix, err := types.CreateStorageKey(c.metadata, SubtensorPrefix, WeightsKey, netuidToBytes(netuid), source[:])
	if err != nil {
		return nil, fmt.Errorf("creating storage prefix: %w", err)
	}

	// Query all keys under this prefix
	keys, err := c.api.RPC.State.GetKeysLatest(prefix)
	if err != nil {
		return nil, fmt.Errorf("querying storage keys: %w", err)
	}

	weights := make([]Weight, 0, len(keys))
	for _, key := range keys {
		var weight types.U16
		ok, err := c.api.RPC.State.GetStorageLatest(key, &weight)
		if err != nil {
			return nil, fmt.Errorf("querying weight storage: %w", err)
		}
		if !ok {
			continue
		}

		// Extract target from key
		var target types.AccountID
		copy(target[:], key[len(key)-32:])
		weights = append(weights, Weight{
			Source: source,
			Target: target,
			Value:  weight,
			NetUID: netuid,
		})
	}

	return weights, nil
}

// SubscribeValidatorWeights subscribes to weight changes for a validator in a specific subnet
func (c *Client) SubscribeValidatorWeights(netuid types.U16, source types.AccountID) (<-chan Weight, error) {
	if !c.connected {
		return nil, fmt.Errorf("client not connected")
	}

	// Generate storage prefix for all weights from this validator
	prefix, err := types.CreateStorageKey(c.metadata, SubtensorPrefix, WeightsKey, netuidToBytes(netuid), source[:])
	if err != nil {
		return nil, fmt.Errorf("creating storage prefix: %w", err)
	}

	// Create channel for weight updates
	ch := make(chan Weight)

	// Subscribe to storage changes
	sub, err := c.api.RPC.State.SubscribeStorageRaw([]types.StorageKey{prefix})
	if err != nil {
		return nil, fmt.Errorf("subscribing to storage: %w", err)
	}

	// Handle updates in a goroutine
	go func() {
		defer sub.Unsubscribe()
		defer close(ch)

		for set := range sub.Chan() {
			for _, change := range set.Changes {
				if !change.HasStorageData {
					continue
				}

				var weight types.U16
				if err := scale.NewDecoder(bytes.NewReader(change.StorageData)).Decode(&weight); err != nil {
					continue
				}

				var target types.AccountID
				copy(target[:], change.StorageKey[len(change.StorageKey)-32:])
				ch <- Weight{
					Source: source,
					Target: target,
					Value:  weight,
					NetUID: netuid,
				}
			}
		}
	}()

	return ch, nil
}
