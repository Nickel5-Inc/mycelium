package metagraph

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// ChainQuerier defines the interface for querying chain state
type ChainQuerier interface {
	// QueryNeuronCount returns the total number of neurons in a subnet
	QueryNeuronCount(ctx context.Context, netuid types.U16) (types.U16, error)

	// QueryValidatorSet returns the list of validator hotkeys in a subnet
	QueryValidatorSet(ctx context.Context, netuid types.U16) ([]types.AccountID, error)

	// QueryStake returns the stake amount for a validator
	QueryStake(ctx context.Context, hotkey types.AccountID) (types.U64, error)

	// QueryWeights returns the weights set by a validator
	QueryWeights(ctx context.Context, netuid types.U16, source types.AccountID) (map[types.AccountID]types.U16, error)

	// QueryAxonInfo returns the IP, port and version for a validator
	QueryAxonInfo(ctx context.Context, netuid types.U16, hotkey types.AccountID) (string, uint16, string, error)

	// QueryBlock returns the current block number
	QueryBlock(ctx context.Context) (types.U64, error)
}

// SyncFromChain updates the metagraph state by querying the chain
func (m *Metagraph) SyncFromChain(ctx context.Context, querier ChainQuerier) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current block
	block, err := querier.QueryBlock(ctx)
	if err != nil {
		return fmt.Errorf("querying block: %w", err)
	}
	m.Block = block

	// Get total neurons
	n, err := querier.QueryNeuronCount(ctx, m.NetUID)
	if err != nil {
		return fmt.Errorf("querying neuron count: %w", err)
	}
	m.N = n

	// Get validator set
	validators, err := querier.QueryValidatorSet(ctx, m.NetUID)
	if err != nil {
		return fmt.Errorf("querying validator set: %w", err)
	}
	m.Hotkeys = validators

	// Reset maps
	m.Stakes = make(map[types.AccountID]types.U64)
	m.Active = make(map[types.AccountID]bool)
	m.Weights = make(map[types.AccountID]map[types.AccountID]types.U16)
	m.IPs = make(map[types.AccountID]string)
	m.Ports = make(map[types.AccountID]uint16)
	m.Versions = make(map[types.AccountID]string)

	// Query stake and axon info for each validator
	for _, hotkey := range validators {
		// Get stake
		stake, err := querier.QueryStake(ctx, hotkey)
		if err != nil {
			return fmt.Errorf("querying stake for %s: %w", hotkey, err)
		}
		m.Stakes[hotkey] = stake

		// Get weights
		weights, err := querier.QueryWeights(ctx, m.NetUID, hotkey)
		if err != nil {
			return fmt.Errorf("querying weights for %s: %w", hotkey, err)
		}
		m.Weights[hotkey] = weights

		// Get axon info
		ip, port, version, err := querier.QueryAxonInfo(ctx, m.NetUID, hotkey)
		if err != nil {
			return fmt.Errorf("querying axon info for %s: %w", hotkey, err)
		}
		m.IPs[hotkey] = ip
		m.Ports[hotkey] = port
		m.Versions[hotkey] = version

		// Update last update time
		m.LastUpdate[hotkey] = time.Now()

		// Mark as active if we got valid axon info
		m.Active[hotkey] = ip != "" && port != 0
	}

	return nil
}
