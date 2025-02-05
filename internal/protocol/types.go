package protocol

import (
	"time"
)

// MessageType represents the type of a protocol message
type MessageType int

// Message type constants
const (
	MessageTypeUnknown MessageType = iota - 1
	MessageTypeHandshake
	MessageTypeHandshakeResponse
	MessageTypePing
	MessageTypePong
	MessageTypeGossip
	MessageTypeSync
	MessageTypeDBSyncReq
	MessageTypeDBSyncResp
	MessageTypeBlacklistSync
)

// PeerInfo represents information about a peer
type PeerInfo struct {
	ID       string            `json:"id"`
	IP       string            `json:"ip"`
	Port     uint16            `json:"port"`
	Version  string            `json:"version"`
	LastSeen int64             `json:"last_seen"`
	Metadata map[string]string `json:"metadata"`
}

// DBChange represents a database change
type DBChange struct {
	ID        int64  `json:"id"`
	TableName string `json:"table_name"`
	Operation string `json:"operation"`
	RecordID  string `json:"record_id"`
	Data      []byte `json:"data"`
	Timestamp int64  `json:"timestamp"`
	NodeID    string `json:"node_id"`
}

// DBSyncRequest represents a database sync request
type DBSyncRequest struct {
	LastSync int64    `json:"last_sync"`
	Tables   []string `json:"tables"`
}

// DBSyncResponse represents a database sync response
type DBSyncResponse struct {
	Changes []DBChange `json:"changes"`
}

// NodeStatus represents the current status of a node
type NodeStatus struct {
	ID              string        `json:"id"`
	Version         string        `json:"version"`
	StartTime       time.Time     `json:"start_time"`
	LastSync        time.Time     `json:"last_sync"`
	LastHeartbeat   time.Time     `json:"last_heartbeat"`
	IsActive        bool          `json:"is_active"`
	Status          string        `json:"status"`
	PeerCount       int           `json:"peer_count"`
	ServingRate     float64       `json:"serving_rate"`
	SyncProgress    float64       `json:"sync_progress"`
	ResponseLatency time.Duration `json:"response_latency"`
	MemoryUsage     uint64        `json:"memory_usage"`
	CPUUsage        float64       `json:"cpu_usage"`
}
