package peer

import (
	"context"
	"log"
	"sync"
	"time"

	"mycelium/internal/protocol"

	"github.com/gorilla/websocket"
)

type Peer struct {
	conn    *websocket.Conn
	send    chan []byte
	mu      sync.Mutex
	info    protocol.PeerInfo
	manager *PeerManager
}

func NewPeer(conn *websocket.Conn, manager *PeerManager) *Peer {
	return &Peer{
		conn:    conn,
		send:    make(chan []byte, 256),
		manager: manager,
	}
}

func (p *Peer) handleMessage(_ int, data []byte) {
	msg, err := protocol.DecodeMessage(data)
	if err != nil {
		log.Printf("Failed to decode message: %v", err)
		return
	}

	switch msg.Type {
	case protocol.MessageTypeGossip:
		p.manager.handleGossip(msg)
	case protocol.MessageTypeSync:
		p.handleSync(msg)
	case protocol.MessageTypeSyncReply:
		p.handleSyncReply(msg)
	case protocol.MessageTypePing:
		p.handlePing(msg)
	case protocol.MessageTypePong:
		p.handlePong(msg)
	case protocol.MessageTypeDBSync, protocol.MessageTypeDBSyncReq, protocol.MessageTypeDBSyncResp:
		p.manager.handleDBSync(msg)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (p *Peer) handlePing(_ *protocol.Message) {
	pong := protocol.NewMessage(protocol.MessageTypePong, p.manager.nodeID, nil)
	encoded, err := pong.Encode()
	if err != nil {
		log.Printf("Failed to encode pong message: %v", err)
		return
	}

	select {
	case p.send <- encoded:
	default:
		log.Printf("Failed to send pong message: buffer full")
	}
}

func (p *Peer) handlePong(_ *protocol.Message) {
	p.mu.Lock()
	p.info.LastSeen = time.Now()
	p.mu.Unlock()
}

func (p *Peer) handleSync(msg *protocol.Message) {
	// Implement sync logic
}

func (p *Peer) handleSyncReply(msg *protocol.Message) {
	// Implement sync reply logic
}

func (p *Peer) startPingLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ping := protocol.NewMessage(protocol.MessageTypePing, p.manager.nodeID, nil)
			encoded, err := ping.Encode()
			if err != nil {
				log.Printf("Failed to encode ping message: %v", err)
				continue
			}

			select {
			case p.send <- encoded:
			default:
				log.Printf("Failed to send ping message: buffer full")
			}
		}
	}
}

func (p *Peer) Handle(ctx context.Context) {
	go p.readPump(ctx)
	go p.writePump(ctx)
	go p.startPingLoop(ctx)
}

func (p *Peer) readPump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			messageType, message, err := p.conn.ReadMessage()
			if err != nil {
				return
			}
			// Handle incoming message
			p.handleMessage(messageType, message)
		}
	}
}

func (p *Peer) writePump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-p.send:
			if err := p.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}
}
