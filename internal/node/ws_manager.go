package node

import (
	"context"
	"log"
	"net/http"
	"sync"

	"mycelium/internal/identity"
	"mycelium/internal/peer"
	"mycelium/internal/substrate"
	"mycelium/internal/util"

	"github.com/gorilla/websocket"
)

// WSManager handles WebSocket connections and their lifecycle
type WSManager struct {
	mu sync.RWMutex

	// Core components
	peers    *peer.PeerManager
	identity *identity.Identity
	netUID   uint16

	// WebSocket configuration
	upgrader websocket.Upgrader
	server   *http.Server

	// Connection tracking
	activeConns sync.WaitGroup
}

// NewWSManager creates a new WebSocket manager
func NewWSManager(peers *peer.PeerManager, identity *identity.Identity, netUID uint16) *WSManager {
	return &WSManager{
		peers:    peers,
		identity: identity,
		netUID:   netUID,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Implement proper origin checking
				return true
			},
		},
	}
}

// Start initializes the WebSocket server
func (wm *WSManager) Start(ctx context.Context, listenAddr string) error {
	// Set up WebSocket handler
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wm.handleWebSocket)

	// Create HTTP server
	wm.server = &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := wm.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// Monitor context for shutdown
	go func() {
		<-ctx.Done()
		if err := wm.Stop(); err != nil {
			log.Printf("Error stopping WebSocket server: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the WebSocket server
func (wm *WSManager) Stop() error {
	// Wait for all active connections to complete
	wm.activeConns.Wait()

	// Shutdown HTTP server
	if wm.server != nil {
		return wm.server.Shutdown(context.Background())
	}
	return nil
}

// handleWebSocket handles incoming WebSocket connections
func (wm *WSManager) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	remoteIP := util.GetRemoteIP(r)
	if wm.peers.IsIPBlocked(remoteIP) {
		http.Error(w, "IP is blocked", http.StatusForbidden)
		return
	}

	conn, err := wm.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Track active connection
	wm.activeConns.Add(1)
	defer func() {
		conn.Close()
		wm.activeConns.Done()
	}()

	// Create a temporary identity for the peer until handshake
	tempWallet, err := substrate.NewWallet("", "") // Empty addresses until handshake
	if err != nil {
		log.Printf("Failed to create temporary wallet: %v", err)
		return
	}

	id := identity.New(tempWallet, wm.netUID)
	id.SetNetworkInfo(remoteIP, 0) // Port will be set during handshake

	peer := peer.NewPeer(conn, wm.peers, id)
	wm.peers.AddPeer(peer)
	defer wm.peers.RemovePeer(peer)

	// Handle peer connection
	peer.Handle(r.Context())
}
