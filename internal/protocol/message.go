package protocol

import (
	"encoding/json"
	"time"
)

const (
	MessageTypeGossip     = "gossip"
	MessageTypeSync       = "sync"
	MessageTypeSyncReply  = "sync_reply"
	MessageTypePing       = "ping"
	MessageTypePong       = "pong"
	MessageTypeDBSync     = "db_sync"
	MessageTypeDBSyncReq  = "db_sync_req"
	MessageTypeDBSyncResp = "db_sync_resp"
)

type Message struct {
	Type      string         `json:"type"`
	SenderID  string         `json:"sender_id"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

type PeerInfo struct {
	ID       string            `json:"id"`
	Address  string            `json:"address"`
	LastSeen time.Time         `json:"last_seen"`
	Version  string            `json:"version"`
	Metadata map[string]string `json:"metadata"`
}

type DBSyncRequest struct {
	LastSync time.Time `json:"last_sync"`
	Tables   []string  `json:"tables"`
}

type DBSyncResponse struct {
	Changes []DBChange `json:"changes"`
}

type DBChange struct {
	ID        int64          `json:"id"`
	TableName string         `json:"table_name"`
	Operation string         `json:"operation"`
	RecordID  string         `json:"record_id"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
	NodeID    string         `json:"node_id"`
}

func NewMessage(msgType, senderID string, payload map[string]any) *Message {
	return &Message{
		Type:      msgType,
		SenderID:  senderID,
		Timestamp: time.Now(),
		Payload:   payload,
	}
}

func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
