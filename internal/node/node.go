package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"mycelium/internal/config"
	"mycelium/internal/database"
	"mycelium/internal/identity"
	"mycelium/internal/metagraph"
	"mycelium/internal/peer"
	"mycelium/internal/protocol"
	"mycelium/internal/substrate"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// generateNodeID creates a random 16-byte node identifier
func generateNodeID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type Node struct {
	mu sync.RWMutex

	// Core components
	ctx        context.Context
	config     *config.Config
	db         database.Database
	peers      *peer.PeerManager
	identity   *identity.Identity
	meta       *metagraph.Metagraph
	querier    metagraph.ChainQuerier
	wsManager  *WSManager
	httpServer *http.Server

	// Node state
	startTime     time.Time
	lastSync      time.Time
	lastHeartbeat time.Time
	isActive      bool
	status        string

	// Performance metrics
	servingRate     float64
	responseLatency time.Duration
	syncProgress    float64
	memoryUsage     uint64
	cpuUsage        float64

	// Prometheus metrics
	metrics *Metrics

	// Channels
	done chan struct{}
}

func NewNode(ctx context.Context, cfg *config.Config) (*Node, error) {
	// Initialize database connection
	db, err := database.New(cfg.ToDBConfig())
	if err != nil {
		return nil, err
	}

	// Initialize substrate components
	wallet, err := substrate.NewWallet(cfg.Hotkey, cfg.Coldkey)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	// Initialize sync manager using hotkey as identifier
	syncMgr, err := database.NewSyncManager(db, cfg.Hotkey)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync manager: %w", err)
	}

	querier, err := metagraph.NewSubstrateQuerier(cfg.SubstrateURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create substrate querier: %w", err)
	}

	meta := metagraph.New(types.U16(cfg.NetUID))
	if err := meta.SyncFromChain(ctx, querier); err != nil {
		return nil, fmt.Errorf("failed to sync metagraph: %w", err)
	}

	// Initialize identity using hotkey
	identity := identity.New(wallet, cfg.NetUID)
	identity.Version = cfg.Version
	identity.SetNetworkInfo(cfg.ListenAddr, cfg.Port)

	// Initialize peer manager
	peers, err := peer.NewPeerManager(
		identity,
		meta,
		querier,
		syncMgr,
		cfg.VerifyURL,
		float64(cfg.MinStake),
		cfg.PortRange,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer manager: %w", err)
	}

	// Initialize WebSocket manager
	wsManager := NewWSManager(peers, identity, cfg.NetUID)

	n := &Node{
		ctx:       ctx,
		config:    cfg,
		db:        db,
		peers:     peers,
		identity:  identity,
		meta:      meta,
		querier:   querier,
		wsManager: wsManager,
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	// Initialize Prometheus metrics
	n.metrics = NewMetrics()

	return n, nil
}

func (n *Node) Start() error {
	// Start WebSocket manager with complete listen address
	listenAddr := fmt.Sprintf("%s:%d", n.config.ListenAddr, n.config.Port)
	if err := n.wsManager.Start(n.ctx, listenAddr); err != nil {
		return fmt.Errorf("failed to start WebSocket manager: %w", err)
	}

	// Start peer discovery
	go n.peers.StartDiscovery(n.ctx)

	return nil
}

func (n *Node) Stop() error {
	// Stop WebSocket manager
	if err := n.wsManager.Stop(); err != nil {
		return fmt.Errorf("failed to stop WebSocket manager: %w", err)
	}

	// Close database connection
	if err := n.db.Close(); err != nil {
		return err
	}

	// Unregister metrics
	n.metrics.Close()

	return nil
}

// UpdateMetrics updates the node's performance metrics
func (n *Node) UpdateMetrics() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Update peer count
	n.metrics.PeerCount.Set(float64(len(n.peers.GetPeers())))

	// Update active validators
	activeValidators := 0
	for _, isActive := range n.meta.Active {
		if isActive {
			activeValidators++
		}
	}
	n.metrics.ActiveValidators.Set(float64(activeValidators))

	// Update serving rate and response latency
	n.metrics.ServingRate.Set(n.servingRate)
	n.metrics.ResponseLatency.Observe(n.responseLatency.Seconds())

	// Update sync progress
	n.metrics.SyncProgress.Set(n.syncProgress)

	// Update resource usage
	n.metrics.MemoryUsage.Set(float64(n.memoryUsage))
	n.metrics.CPUUsage.Set(n.cpuUsage)
}

// GetStatus returns the current status of the node
func (n *Node) GetStatus() protocol.NodeStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return protocol.NodeStatus{
		ID:              n.identity.GetID(),
		Version:         n.identity.Version,
		StartTime:       n.startTime,
		LastSync:        n.lastSync,
		LastHeartbeat:   n.lastHeartbeat,
		IsActive:        n.isActive,
		Status:          n.status,
		PeerCount:       len(n.peers.GetPeers()),
		ServingRate:     n.servingRate,
		SyncProgress:    n.syncProgress,
		ResponseLatency: n.responseLatency,
		MemoryUsage:     n.memoryUsage,
		CPUUsage:        n.cpuUsage,
	}
}
