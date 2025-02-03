package protocol

import "time"

// NodeStatus represents the current status of a node
type NodeStatus struct {
	// Core identification
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	StartTime time.Time `json:"start_time"`

	// State information
	LastSync      time.Time `json:"last_sync"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	IsActive      bool      `json:"is_active"`
	Status        string    `json:"status"`
	PeerCount     int       `json:"peer_count"`

	// Performance metrics
	ServingRate     float64       `json:"serving_rate"`
	SyncProgress    float64       `json:"sync_progress"`
	ResponseLatency time.Duration `json:"response_latency"`
	MemoryUsage     uint64        `json:"memory_usage"`
	CPUUsage        float64       `json:"cpu_usage"`
}
