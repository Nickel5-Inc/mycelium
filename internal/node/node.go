package node

import (
	"context"
	"log"
	"net/http"

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
	n.peers = peer.NewPeerManager(cfg.ListenAddr, db, cfg.VerifyURL)

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
