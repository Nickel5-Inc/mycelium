package mocks

import (
	"context"
	"sync"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MockDatabase implements the Database interface for testing
type MockDatabase struct {
	mu      sync.RWMutex
	data    map[string]interface{}
	queries []string
}

// NewMockDatabase creates a new mock database
func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		data:    make(map[string]interface{}),
		queries: make([]string, 0),
	}
}

// Ping implements Database.Ping
func (m *MockDatabase) Ping(ctx context.Context) error {
	return nil
}

// Close implements Database.Close
func (m *MockDatabase) Close() error {
	return nil
}

// WithTx implements Database.WithTx
func (m *MockDatabase) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return fn(nil) // Mock implementation - doesn't actually use transactions
}

// GetPool implements Database.GetPool
func (m *MockDatabase) GetPool() *pgxpool.Pool {
	return nil
}

// MockMetagraph implements metagraph.ChainQuerier interface for testing
type MockMetagraph struct {
	mu       sync.RWMutex
	neurons  map[types.U16][]types.AccountID
	stakes   map[types.AccountID]types.U64
	weights  map[types.AccountID]map[types.AccountID]types.U16
	axonInfo map[types.AccountID]AxonInfo
	block    types.U64
}

type AxonInfo struct {
	IP      string
	Port    uint16
	Version string
}

func NewMockMetagraph() *MockMetagraph {
	return &MockMetagraph{
		neurons:  make(map[types.U16][]types.AccountID),
		stakes:   make(map[types.AccountID]types.U64),
		weights:  make(map[types.AccountID]map[types.AccountID]types.U16),
		axonInfo: make(map[types.AccountID]AxonInfo),
	}
}

func (m *MockMetagraph) QueryNeuronCount(ctx context.Context, netuid types.U16) (types.U16, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return types.U16(len(m.neurons[netuid])), nil
}

func (m *MockMetagraph) QueryValidatorSet(ctx context.Context, netuid types.U16) ([]types.AccountID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.neurons[netuid], nil
}

func (m *MockMetagraph) QueryStake(ctx context.Context, hotkey types.AccountID) (types.U64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stakes[hotkey], nil
}

func (m *MockMetagraph) QueryWeights(ctx context.Context, netuid types.U16, source types.AccountID) (map[types.AccountID]types.U16, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.weights[source], nil
}

func (m *MockMetagraph) QueryAxonInfo(ctx context.Context, netuid types.U16, hotkey types.AccountID) (string, uint16, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info := m.axonInfo[hotkey]
	return info.IP, info.Port, info.Version, nil
}

func (m *MockMetagraph) QueryBlock(ctx context.Context) (types.U64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.block, nil
}

// Test data setters
func (m *MockMetagraph) SetNeurons(netuid types.U16, neurons []types.AccountID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.neurons[netuid] = neurons
}

func (m *MockMetagraph) SetStake(hotkey types.AccountID, stake types.U64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stakes[hotkey] = stake
}

func (m *MockMetagraph) SetWeights(source types.AccountID, weights map[types.AccountID]types.U16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.weights[source] = weights
}

func (m *MockMetagraph) SetAxonInfo(hotkey types.AccountID, ip string, port uint16, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.axonInfo[hotkey] = AxonInfo{
		IP:      ip,
		Port:    port,
		Version: version,
	}
}

func (m *MockMetagraph) SetBlock(block types.U64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.block = block
}

// MockPeerManager implements a mock peer manager for testing
type MockPeerManager struct {
	mu    sync.RWMutex
	peers map[string]struct{}
}

func NewMockPeerManager() *MockPeerManager {
	return &MockPeerManager{
		peers: make(map[string]struct{}),
	}
}

func (m *MockPeerManager) AddPeer(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers[id] = struct{}{}
}

func (m *MockPeerManager) RemovePeer(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.peers, id)
}

func (m *MockPeerManager) GetPeers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	peers := make([]string, 0, len(m.peers))
	for id := range m.peers {
		peers = append(peers, id)
	}
	return peers
}

// MockNode implements a mock node for testing
type MockNode struct {
	mu       sync.RWMutex
	isActive bool
	status   string
	metrics  map[string]float64
}

func NewMockNode() *MockNode {
	return &MockNode{
		isActive: true,
		metrics:  make(map[string]float64),
	}
}

func (m *MockNode) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isActive = true
	return nil
}

func (m *MockNode) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isActive = false
	return nil
}

func (m *MockNode) IsActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isActive
}

func (m *MockNode) SetMetric(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics[name] = value
}

func (m *MockNode) GetMetric(name string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.metrics[name]
}

// MockValidator implements a mock validator for testing
type MockValidator struct {
	mu     sync.RWMutex
	stake  float64
	active bool
}

func NewMockValidator(stake float64) *MockValidator {
	return &MockValidator{
		stake:  stake,
		active: true,
	}
}

func (m *MockValidator) GetStake() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stake
}

func (m *MockValidator) IsActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

func (m *MockValidator) SetActive(active bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = active
}

func (m *MockValidator) SetStake(stake float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stake = stake
}
