package peer

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"mycelium/internal/identity"
	"mycelium/internal/protocol"
	"mycelium/internal/util"

	"github.com/gorilla/websocket"
)

// Connection constants
const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512 * 1024 // 512KB
)

// Message type constants
const (
	MessageTypeGossip        = "gossip"
	MessageTypeSync          = "sync"
	MessageTypeHandshake     = "handshake"
	MessageTypeBlacklistSync = "blacklist_sync"
	MessageTypeDBSyncReq     = "db_sync_req"
	MessageTypeDBSyncResp    = "db_sync_resp"
)

// Peer represents a connected peer node
type Peer struct {
	conn     *websocket.Conn
	send     chan []byte
	manager  *PeerManager
	identity *identity.Identity
	mu       sync.RWMutex
}

// NewPeer creates a new peer instance
func NewPeer(conn *websocket.Conn, manager *PeerManager, id *identity.Identity) *Peer {
	return &Peer{
		conn:     conn,
		send:     make(chan []byte, 256),
		manager:  manager,
		identity: id,
	}
}

// ID returns the peer's identifier (hotkey address)
func (p *Peer) ID() string {
	return p.identity.GetID()
}

// Identity returns the peer's identity information
func (p *Peer) Identity() *identity.Identity {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.identity
}

// Handle manages the peer connection
func (p *Peer) Handle(ctx context.Context) {
	// Send initial handshake
	payload := map[string]interface{}{
		"hotkey":  p.manager.identity.GetID(),
		"version": p.manager.identity.Version,
	}

	msg, err := protocol.NewMessage(protocol.MessageTypeHandshake, p.manager.identity.GetID(), payload)
	if err != nil {
		log.Printf("Failed to create handshake message: %v", err)
		return
	}

	data, err := msg.Encode()
	if err != nil {
		log.Printf("Failed to encode handshake: %v", err)
		return
	}

	select {
	case p.send <- data:
	default:
		log.Printf("Failed to send handshake: buffer full")
		return
	}

	// Start read/write pumps
	go p.readPump(ctx)
	go p.writePump(ctx)
	go p.startPingLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()
}

// handleMessage processes an incoming message from a peer
func (p *Peer) handleMessage(data []byte) error {
	msg, err := protocol.DecodeMessage(data)
	if err != nil {
		return fmt.Errorf("failed to decode message: %w", err)
	}

	// Pass all messages to manager
	p.manager.handlePeerMsg(p, msg)
	return nil
}

// handleGossipMsg processes a gossip message
func (p *Peer) handleGossipMsg(msg *protocol.Message) error {
	p.manager.handleGossip(msg)
	return nil
}

// handleSync processes a sync message
func (p *Peer) handleSync(msg *protocol.Message) error {
	// Create sync response
	payload := map[string]interface{}{
		"peers": p.manager.GetPeers(),
	}

	resp, err := protocol.NewMessage(protocol.MessageTypeSync, p.manager.identity.GetID(), payload)
	if err != nil {
		return fmt.Errorf("creating sync response: %w", err)
	}

	data, err := resp.Encode()
	if err != nil {
		return fmt.Errorf("encoding sync response: %w", err)
	}

	select {
	case p.send <- data:
		return nil
	default:
		return fmt.Errorf("failed to send sync response: buffer full")
	}
}

// handleBlacklistSync processes a blacklist sync message
func (p *Peer) handleBlacklistSync(msg *protocol.Message) error {
	syncData, ok := msg.Payload["blacklist_sync"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid blacklist sync payload")
	}

	// Convert directly to blacklist types
	var update struct {
		Greylist map[string]struct {
			IP             string    `json:"ip"`
			FailedAttempts int       `json:"failed_attempts"`
			FirstFailure   time.Time `json:"first_failure"`
			LastFailure    time.Time `json:"last_failure"`
			GreylistCount  int       `json:"greylist_count"`
			BlacklistedAt  time.Time `json:"blacklisted_at"`
			LastStakeCheck float64   `json:"last_stake_check"`
		} `json:"greylist"`
		Blacklist map[string]struct {
			IP             string    `json:"ip"`
			FailedAttempts int       `json:"failed_attempts"`
			FirstFailure   time.Time `json:"first_failure"`
			LastFailure    time.Time `json:"last_failure"`
			GreylistCount  int       `json:"greylist_count"`
			BlacklistedAt  time.Time `json:"blacklisted_at"`
			LastStakeCheck float64   `json:"last_stake_check"`
		} `json:"blacklist"`
	}

	if err := util.ConvertMapToStruct(syncData, &update); err != nil {
		return fmt.Errorf("failed to decode blacklist sync: %w", err)
	}

	// Convert to blacklist manager format
	blacklistUpdate := &SyncUpdate{
		Greylist:  make(map[string]IPStatus),
		Blacklist: make(map[string]IPStatus),
	}

	// Convert greylist
	for ip, status := range update.Greylist {
		blacklistUpdate.Greylist[ip] = IPStatus{
			IP:             status.IP,
			FailedAttempts: status.FailedAttempts,
			FirstFailure:   status.FirstFailure,
			LastFailure:    status.LastFailure,
			GreylistCount:  status.GreylistCount,
			BlacklistedAt:  status.BlacklistedAt,
			LastStakeCheck: status.LastStakeCheck,
		}
	}

	// Convert blacklist
	for ip, status := range update.Blacklist {
		blacklistUpdate.Blacklist[ip] = IPStatus{
			IP:             status.IP,
			FailedAttempts: status.FailedAttempts,
			FirstFailure:   status.FirstFailure,
			LastFailure:    status.LastFailure,
			GreylistCount:  status.GreylistCount,
			BlacklistedAt:  status.BlacklistedAt,
			LastStakeCheck: status.LastStakeCheck,
		}
	}

	// Apply the update to our blacklist manager
	p.manager.blacklist.ApplySyncUpdate(blacklistUpdate)
	return nil
}

// readPump pumps messages from the websocket connection to the hub
func (p *Peer) readPump(ctx context.Context) {
	defer func() {
		p.conn.Close()
		close(p.send)
		p.manager.RemovePeer(p)
	}()

	p.conn.SetReadLimit(maxMessageSize)
	p.conn.SetReadDeadline(time.Now().Add(pongWait))
	p.conn.SetPongHandler(func(string) error {
		p.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := p.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Error reading from peer %s: %v", p.ID(), err)
				}
				return
			}

			if err := p.handleMessage(message); err != nil {
				log.Printf("Error handling message from peer %s: %v", p.ID(), err)
				return
			}
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
func (p *Peer) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		p.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case message, ok := <-p.send:
			p.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed
				p.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := p.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			_, err = w.Write(message)
			if err != nil {
				return
			}

			// Add queued messages
			n := len(p.send)
			for i := 0; i < n; i++ {
				_, err = w.Write([]byte{'\n'})
				if err != nil {
					return
				}
				msg := <-p.send
				_, err = w.Write(msg)
				if err != nil {
					return
				}
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			p.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := p.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// startPingLoop sends periodic ping messages to keep the connection alive
func (p *Peer) startPingLoop(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.sendPing(); err != nil {
				log.Printf("Failed to send ping to peer %s: %v", p.ID(), err)
			}
		}
	}
}

// sendPing sends a ping message to the peer
func (p *Peer) sendPing() error {
	payload := map[string]interface{}{
		"ping": time.Now().UnixNano(),
	}

	msg, err := protocol.NewMessage(protocol.MessageTypeSync, p.manager.identity.GetID(), payload)
	if err != nil {
		return fmt.Errorf("creating ping message: %w", err)
	}

	encoded, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encoding ping message: %w", err)
	}

	select {
	case p.send <- encoded:
		return nil
	default:
		return fmt.Errorf("failed to send ping: buffer full")
	}
}

func (p *Peer) syncPeers() error {
	// Create sync request
	payload := map[string]interface{}{
		"request_full_sync": true,
	}

	msg, err := protocol.NewMessage(protocol.MessageTypeSync, p.manager.identity.GetID(), payload)
	if err != nil {
		return fmt.Errorf("creating sync request: %w", err)
	}

	data, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encoding sync request: %w", err)
	}

	select {
	case p.send <- data:
		return nil
	default:
		return fmt.Errorf("failed to send sync request: buffer full")
	}
}
