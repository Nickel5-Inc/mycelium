package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"mycelium/internal/config"
	"mycelium/internal/database"
	"mycelium/internal/peer"

	"github.com/gorilla/websocket"
)

type Node struct {
	ctx        context.Context
	config     *config.Config
	db         database.Database
	peers      *peer.PeerManager
	httpServer *http.Server
	upgrader   websocket.Upgrader
}

func NewNode(ctx context.Context, cfg *config.Config) (*Node, error) {
	n := &Node{
		ctx:    ctx,
		config: cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Implement proper origin checking
				return true
			},
		},
	}

	// Initialize database connection
	db, err := database.New(cfg.Database)
	if err != nil {
		return nil, err
	}
	n.db = db

	// Initialize peer manager
	peers, err := peer.NewPeerManager(cfg.ListenAddr, cfg.PortRange, db, cfg.VerifyURL, cfg.MinStake, cfg.PortRotation)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer manager: %w", err)
	}
	n.peers = peers

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

	peer := peer.NewPeer(conn, n.peers)
	n.peers.AddPeer(peer)
	defer n.peers.RemovePeer(peer)

	// Handle peer connection
	peer.Handle(n.ctx)
}

// getRemoteIP extracts the remote IP from a request
func getRemoteIP(r *http.Request) string {
	// Check X-Real-IP header first (for proxies)
	ip := r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}

	// Check X-Forwarded-For header
	ip = r.Header.Get("X-Forwarded-For")
	if ip != "" {
		// X-Forwarded-For can contain multiple IPs, use the first one
		ips := strings.Split(ip, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
