package metagraph

import (
	"context"
	"encoding/binary"
	"fmt"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// SubstrateQuerier implements ChainQuerier using the Substrate RPC client
type SubstrateQuerier struct {
	api *gsrpc.SubstrateAPI
}

// NewSubstrateQuerier creates a new SubstrateQuerier
func NewSubstrateQuerier(endpoint string) (*SubstrateQuerier, error) {
	api, err := gsrpc.NewSubstrateAPI(endpoint)
	if err != nil {
		return nil, fmt.Errorf("creating substrate API: %w", err)
	}

	// Get metadata
	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		return nil, fmt.Errorf("getting metadata: %w", err)
	}

	// Verify Subtensor module exists by checking TotalIssuance storage
	_, err = types.CreateStorageKey(meta, "SubtensorModule", "TotalIssuance")
	if err != nil {
		return nil, fmt.Errorf("Subtensor module not found in chain metadata: %w", err)
	}

	return &SubstrateQuerier{api: api}, nil
}

// QueryNeuronCount returns the total number of neurons in a subnet
func (q *SubstrateQuerier) QueryNeuronCount(ctx context.Context, netuid types.U16) (types.U16, error) {
	key, err := createStorageKey(q.api, "SubtensorModule", "TotalIssuance", netuidToBytes(netuid))
	if err != nil {
		return 0, fmt.Errorf("creating storage key: %w", err)
	}
	var count types.U16
	ok, err := q.api.RPC.State.GetStorageLatest(key, &count)
	if err != nil {
		return 0, fmt.Errorf("querying neuron count: %w", err)
	}
	if !ok {
		return 0, fmt.Errorf("neuron count not found for netuid %d", netuid)
	}
	return count, nil
}

// QueryValidatorSet returns the list of validator hotkeys in a subnet
func (q *SubstrateQuerier) QueryValidatorSet(ctx context.Context, netuid types.U16) ([]types.AccountID, error) {
	key, err := createStorageKey(q.api, "SubtensorModule", "ValidatorSet", netuidToBytes(netuid))
	if err != nil {
		return nil, fmt.Errorf("creating storage key: %w", err)
	}
	var validators []types.AccountID
	ok, err := q.api.RPC.State.GetStorageLatest(key, &validators)
	if err != nil {
		return nil, fmt.Errorf("querying validator set: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("validator set not found for netuid %d", netuid)
	}
	return validators, nil
}

// QueryStake returns the stake amount for a validator
func (q *SubstrateQuerier) QueryStake(ctx context.Context, hotkey types.AccountID) (types.U64, error) {
	key, err := createStorageKey(q.api, "SubtensorModule", "Stake", hotkey[:])
	if err != nil {
		return 0, fmt.Errorf("creating storage key: %w", err)
	}
	var stake types.U64
	ok, err := q.api.RPC.State.GetStorageLatest(key, &stake)
	if err != nil {
		return 0, fmt.Errorf("querying stake: %w", err)
	}
	if !ok {
		return 0, nil // No stake is equivalent to 0 stake
	}
	return stake, nil
}

// QueryWeights returns the weights set by a validator
func (q *SubstrateQuerier) QueryWeights(ctx context.Context, netuid types.U16, source types.AccountID) (map[types.AccountID]types.U16, error) {
	// Create storage key for weights
	key, err := createStorageKey(q.api, "SubtensorModule", "Weights", uint16ToBytes(uint16(netuid)), source[:])
	if err != nil {
		return nil, fmt.Errorf("creating storage key: %w", err)
	}

	var weights map[types.AccountID]types.U16
	ok, err := q.api.RPC.State.GetStorageLatest(key, &weights)
	if err != nil {
		return nil, fmt.Errorf("querying weights: %w", err)
	}
	if !ok {
		return make(map[types.AccountID]types.U16), nil // No weights is equivalent to empty map
	}
	return weights, nil
}

// QueryAxonInfo returns the IP, port and version for a validator
func (q *SubstrateQuerier) QueryAxonInfo(ctx context.Context, netuid types.U16, hotkey types.AccountID) (string, uint16, string, error) {
	key, err := createStorageKey(q.api, "SubtensorModule", "AxonInfo", uint16ToBytes(uint16(netuid)), hotkey[:])
	if err != nil {
		return "", 0, "", fmt.Errorf("creating storage key: %w", err)
	}

	type AxonInfo struct {
		IP      string
		Port    types.U16
		Version string
	}

	var info AxonInfo
	ok, err := q.api.RPC.State.GetStorageLatest(key, &info)
	if err != nil {
		return "", 0, "", fmt.Errorf("querying axon info: %w", err)
	}
	if !ok {
		return "", 0, "", nil // No axon info is equivalent to empty values
	}
	return info.IP, uint16(info.Port), info.Version, nil
}

// QueryBlock returns the current block number
func (q *SubstrateQuerier) QueryBlock(ctx context.Context) (types.U64, error) {
	header, err := q.api.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return 0, fmt.Errorf("querying latest header: %w", err)
	}
	return types.U64(header.Number), nil
}

// Helper function to create storage keys
func createStorageKey(api *gsrpc.SubstrateAPI, module, function string, args ...[]byte) (types.StorageKey, error) {
	metadata, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		return nil, fmt.Errorf("getting metadata: %w", err)
	}

	key, err := types.CreateStorageKey(metadata, module, function)
	if err != nil {
		return nil, fmt.Errorf("creating storage key: %w", err)
	}

	for _, arg := range args {
		key = append(key, arg...)
	}
	return key, nil
}

// Helper function to convert uint16 to bytes
func uint16ToBytes(n uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, n)
	return buf
}

// Helper function to convert netuid to bytes
func netuidToBytes(netuid types.U16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(netuid))
	return buf
}
