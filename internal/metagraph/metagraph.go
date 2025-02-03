package metagraph

import (
	"sync"
	"time"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// Metagraph represents the state of the network
type Metagraph struct {
	mu sync.RWMutex

	// Network parameters
	NetUID types.U16
	N      types.U16 // Number of neurons
	Block  types.U64 // Current block

	// Validator information
	Hotkeys    []types.AccountID // List of validator hotkeys
	Stakes     map[types.AccountID]types.U64
	Ranks      map[types.AccountID]types.U16
	Trust      map[types.AccountID]types.U16
	Active     map[types.AccountID]bool
	LastUpdate map[types.AccountID]time.Time

	// Weight matrix
	Weights map[types.AccountID]map[types.AccountID]types.U16 // [source][target]weight

	// Axon information
	IPs      map[types.AccountID]string
	Ports    map[types.AccountID]uint16
	Versions map[types.AccountID]string
}

// New creates a new Metagraph instance
func New(netuid types.U16) *Metagraph {
	return &Metagraph{
		NetUID:     netuid,
		Stakes:     make(map[types.AccountID]types.U64),
		Ranks:      make(map[types.AccountID]types.U16),
		Trust:      make(map[types.AccountID]types.U16),
		Active:     make(map[types.AccountID]bool),
		LastUpdate: make(map[types.AccountID]time.Time),
		Weights:    make(map[types.AccountID]map[types.AccountID]types.U16),
		IPs:        make(map[types.AccountID]string),
		Ports:      make(map[types.AccountID]uint16),
		Versions:   make(map[types.AccountID]string),
	}
}

// Sync updates the metagraph state from the chain
func (m *Metagraph) Sync() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// TODO: Implement chain syncing
	// 1. Query total neurons (N)
	// 2. Query validator set
	// 3. Query stakes
	// 4. Query weights
	// 5. Query axon info
	// 6. Update last sync time

	return nil
}

// GetWeight returns the weight from source to target
func (m *Metagraph) GetWeight(source, target types.AccountID) types.U16 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if weights, ok := m.Weights[source]; ok {
		return weights[target]
	}
	return 0
}

// GetStake returns the stake for a validator
func (m *Metagraph) GetStake(hotkey types.AccountID) types.U64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Stakes[hotkey]
}

// GetAxonInfo returns the IP, port and version for a validator
func (m *Metagraph) GetAxonInfo(hotkey types.AccountID) (string, uint16, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.IPs[hotkey], m.Ports[hotkey], m.Versions[hotkey]
}

// IsActive returns whether a validator is active
func (m *Metagraph) IsActive(hotkey types.AccountID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Active[hotkey]
}

// GetValidators returns the list of validator hotkeys
func (m *Metagraph) GetValidators() []types.AccountID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]types.AccountID{}, m.Hotkeys...)
}
