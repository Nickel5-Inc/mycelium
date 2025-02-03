package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"

	"mycelium/internal/config"
	"mycelium/internal/database"
	"mycelium/internal/identity"
	"mycelium/internal/metagraph"
	"mycelium/internal/peer"
	"mycelium/internal/substrate"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/gorilla/websocket"
)

// generateNodeID creates a random 16-byte node identifier
func generateNodeID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type Node struct {
	ctx        context.Context
	config     *config.Config
	db         database.Database
	peers      *peer.PeerManager
	httpServer *http.Server
	upgrader   websocket.Upgrader
}

func NewNode(ctx context.Context, cfg *config.Config) (*Node, error) {
	// Initialize database connection
	db, err := database.New(cfg.Database)
	if err != nil {
		return nil, err
	}

	// Initialize sync manager
	syncMgr, err := database.NewSyncManager(db, generateNodeID())
	if err != nil {
		return nil, fmt.Errorf("failed to create sync manager: %w", err)
	}

	// Initialize substrate components
	wallet, err := substrate.NewWallet(cfg.Hotkey, cfg.Coldkey)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	querier, err := metagraph.NewSubstrateQuerier("ws://localhost:9944") // TODO: Make configurable
	if err != nil {
		return nil, fmt.Errorf("failed to create substrate querier: %w", err)
	}

	meta := metagraph.New(types.U16(cfg.NetUID))
	if err := meta.SyncFromChain(ctx, querier); err != nil {
		return nil, fmt.Errorf("failed to sync metagraph: %w", err)
	}

	// Initialize peer manager
	peers, err := peer.NewPeerManager(
		identity.New(generateNodeID(), wallet, cfg.NetUID),
		meta,
		querier,
		syncMgr,
		cfg.VerifyURL,
		cfg.MinStake,
		cfg.PortRange,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer manager: %w", err)
	}

	n := &Node{
		ctx:    ctx,
		config: cfg,
		db:     db,
		peers:  peers,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Implement proper origin checking
				return true
			},
		},
	}

	return n, nil
}

func (n *Node) Start() error {
	// Set up WebSocket handler
	http.HandleFunc("/ws", n.handleWebSocket)

	// Start HTTP server
	n.httpServer = &http.Server{
		Addr: n.config.ListenAddr,
	}

	// Start server in goroutine
	go func() {
		if err := n.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start peer discovery
	go n.peers.StartDiscovery(n.ctx)

	return nil
}

func (n *Node) Stop() error {
	// Shutdown HTTP server
	if err := n.httpServer.Shutdown(n.ctx); err != nil {
		return err
	}

	// Close database connection
	if err := n.db.Close(); err != nil {
		return err
	}

	return nil
}

// getRemoteIP extracts the remote IP address from the request
func getRemoteIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP if multiple are present
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // Return as is if no port
	}
	return ip
}

func (n *Node) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	remoteIP := getRemoteIP(r)
	if n.peers.IsIPBlocked(remoteIP) {
		http.Error(w, "IP is blocked", http.StatusForbidden)
		return
	}

	conn, err := n.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Create a temporary identity for the peer until handshake
	tempID := generateNodeID()
	id := identity.New(tempID, nil, n.config.NetUID)
	id.SetNetworkInfo(remoteIP, 0) // Port will be set during handshake

	peer := peer.NewPeer(conn, n.peers, id)
	n.peers.AddPeer(peer)
	defer n.peers.RemovePeer(peer)

	// Handle peer connection
	peer.Handle(n.ctx)
}
