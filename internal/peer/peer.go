package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"mycelium/internal/protocol"

	"github.com/gorilla/websocket"
)

// Peer represents a connected peer node
type Peer struct {
	conn    *websocket.Conn
	send    chan []byte
	manager *PeerManager
	info    protocol.PeerInfo
	mu      sync.RWMutex
}

// NewPeer creates a new peer instance
func NewPeer(conn *websocket.Conn, manager *PeerManager) *Peer {
	return &Peer{
		conn:    conn,
		send:    make(chan []byte, 256),
		manager: manager,
	}
}

// Handle manages the peer connection
func (p *Peer) Handle(ctx context.Context) {
	go p.startPingLoop(ctx)
	go p.writePump(ctx)
	p.readPump(ctx)
}

// handleMessage processes an incoming message from a peer
func (p *Peer) handleMessage(data []byte) error {
	msg, err := protocol.DecodeMessage(data)
	if err != nil {
		return fmt.Errorf("failed to decode message: %v", err)
	}

	switch msg.Type {
	case protocol.MessageTypeGossip:
		return p.handleGossipMsg(msg)
	case protocol.MessageTypeSync:
		return p.handleSyncMsg(msg)
	case protocol.MessageTypeHandshake:
		return p.handleHandshakeMsg(msg)
	case protocol.MessageTypeBlacklistSync:
		return p.handleBlacklistSync(msg)
	case protocol.MessageTypeDBSyncReq, protocol.MessageTypeDBSyncResp:
		p.manager.handleDBSync(msg)
		return nil
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

// handleGossipMsg processes a gossip message
func (p *Peer) handleGossipMsg(msg *protocol.Message) error {
	return p.manager.handleGossip(msg)
}

// handleSyncMsg processes a sync message
func (p *Peer) handleSyncMsg(msg *protocol.Message) error {
	if requestFullSync, ok := msg.Payload["request_full_sync"].(bool); ok && requestFullSync {
		// Send our current peer info
		p.manager.mu.RLock()
		peerInfoList := make([]protocol.PeerInfo, 0, len(p.manager.peerInfo))
		for _, info := range p.manager.peerInfo {
			peerInfoList = append(peerInfoList, info)
		}
		p.manager.mu.RUnlock()

		// Send response
		response := protocol.NewMessage(protocol.MessageTypeSync, p.manager.nodeID, map[string]any{
			"peers": peerInfoList,
		})
		data, err := response.Encode()
		if err != nil {
			return fmt.Errorf("failed to encode sync response: %v", err)
		}
		p.send <- data
	}
	return nil
}

// handleHandshakeMsg processes a handshake message
func (p *Peer) handleHandshakeMsg(msg *protocol.Message) error {
	peerID, ok := msg.Payload["peer_id"].(string)
	if !ok {
		return fmt.Errorf("invalid handshake: missing peer_id")
	}

	signature, ok := msg.Payload["signature"].([]byte)
	if !ok {
		return fmt.Errorf("invalid handshake: missing signature")
	}

	// Verify the peer
	valid, err := p.manager.validators.VerifyValidator(context.Background(), peerID, signature, nil)
	if err != nil {
		return fmt.Errorf("validation failed: %v", err)
	}
	if !valid {
		return fmt.Errorf("invalid validator")
	}

	p.mu.Lock()
	p.info.ID = peerID
	p.info.LastSeen = time.Now()
	p.mu.Unlock()

	return nil
}

// handleBlacklistSync processes a blacklist sync message
func (p *Peer) handleBlacklistSync(msg *protocol.Message) error {
	syncData, ok := msg.Payload["blacklist_sync"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid blacklist sync payload")
	}

	// Convert the map to a SyncUpdate
	update := &SyncUpdate{}

	// Convert greylist
	if greylist, ok := syncData["greylist"].(map[string]any); ok {
		update.Greylist = make(map[string]IPStatus)
		for ip, data := range greylist {
			if statusData, ok := data.(map[string]any); ok {
				status := IPStatus{}
				if err := mapToStruct(statusData, &status); err != nil {
					log.Printf("Error converting greylist status: %v", err)
					continue
				}
				update.Greylist[ip] = status
			}
		}
	}

	// Convert blacklist
	if blacklist, ok := syncData["blacklist"].(map[string]any); ok {
		update.Blacklist = make(map[string]IPStatus)
		for ip, data := range blacklist {
			if statusData, ok := data.(map[string]any); ok {
				status := IPStatus{}
				if err := mapToStruct(statusData, &status); err != nil {
					log.Printf("Error converting blacklist status: %v", err)
					continue
				}
				update.Blacklist[ip] = status
			}
		}
	}

	// Apply the update to our blacklist manager
	p.manager.blacklist.ApplySyncUpdate(update)
	return nil
}

// Helper function to convert a map to a struct
func mapToStruct(m map[string]any, v any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// readPump pumps messages from the websocket connection to the hub
func (p *Peer) readPump(ctx context.Context) {
	defer func() {
		p.manager.RemovePeer(p)
		p.conn.Close()
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
					log.Printf("error reading message: %v", err)
				}
				return
			}
			if err := p.handleMessage(message); err != nil {
				log.Printf("error handling message: %v", err)
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
				p.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := p.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

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

// startPingLoop sends periodic pings to keep the connection alive
func (p *Peer) startPingLoop(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.sendPing(); err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

// sendPing sends a ping message to the peer
func (p *Peer) sendPing() error {
	msg := protocol.NewMessage(protocol.MessageTypeSync, p.manager.nodeID, nil)
	data, err := msg.Encode()
	if err != nil {
		return err
	}

	select {
	case p.send <- data:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024
)
