package protocol

import (
	"encoding/json"
	"time"
)

// Message types for peer communication
const (
	MessageTypeGossip        = "gossip"
	MessageTypeSync          = "sync"
	MessageTypeHandshake     = "handshake"
	MessageTypeBlacklistSync = "blacklist_sync"
	MessageTypeDBSyncReq     = "db_sync_req"
	MessageTypeDBSyncResp    = "db_sync_resp"
)

// Message represents a protocol message between peers
type Message struct {
	Type      string         `json:"type"`
	SenderID  string         `json:"sender_id"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

// PeerInfo represents information about a peer
type PeerInfo struct {
	ID       string            `json:"id"`
	Address  string            `json:"address"`
	LastSeen time.Time         `json:"last_seen"`
	Version  string            `json:"version"`
	Metadata map[string]string `json:"metadata"`
}

// DBSyncRequest represents a database sync request
type DBSyncRequest struct {
	LastSync time.Time `json:"last_sync"`
	Tables   []string  `json:"tables"`
}

// DBSyncResponse represents a database sync response
type DBSyncResponse struct {
	Changes []DBChange `json:"changes"`
}

// DBChange represents a database change record
type DBChange struct {
	ID        int64          `json:"id"`
	TableName string         `json:"table_name"`
	Operation string         `json:"operation"`
	RecordID  string         `json:"record_id"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
	NodeID    string         `json:"node_id"`
}

// NewMessage creates a new protocol message
func NewMessage(msgType string, senderID string, payload map[string]any) *Message {
	return &Message{
		Type:      msgType,
		SenderID:  senderID,
		Timestamp: time.Now(),
		Payload:   payload,
	}
}

// Encode serializes a message to JSON
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage deserializes a message from JSON
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
