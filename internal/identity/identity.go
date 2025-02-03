package identity

import (
	"time"

	"mycelium/internal/substrate"
)

// Identity represents a node's identity in the Mycelium network
type Identity struct {
	// Core identification
	Hotkey  *substrate.Wallet // Validator hotkey (primary identifier)
	NetUID  uint16            // Network/subnet identifier
	Version string            // Software version

	// Network information
	IP       string // IP address
	Port     uint16 // Primary port
	LastSeen time.Time

	// Validator status
	IsActive bool
	Stake    float64
	Rank     int64

	// Additional metadata
	Metadata map[string]string
}

// New creates a new identity
func New(hotkey *substrate.Wallet, netUID uint16) *Identity {
	return &Identity{
		Hotkey:   hotkey,
		NetUID:   netUID,
		Version:  "1.0.0", // TODO: Make configurable
		Metadata: make(map[string]string),
	}
}

// GetID returns the node's identifier (hotkey address)
func (i *Identity) GetID() string {
	if i.Hotkey == nil {
		return ""
	}
	return i.Hotkey.GetAddress()
}

// IsValidator returns whether this identity represents a validator
func (i *Identity) IsValidator() bool {
	return i.Hotkey != nil && i.Stake > 0
}

// UpdateLastSeen updates the last seen timestamp
func (i *Identity) UpdateLastSeen() {
	i.LastSeen = time.Now()
}

// SetNetworkInfo sets the network connection information
func (i *Identity) SetNetworkInfo(ip string, port uint16) {
	i.IP = ip
	i.Port = port
}

// SetValidatorStatus updates the validator status information
func (i *Identity) SetValidatorStatus(isActive bool, stake float64, rank int64) {
	i.IsActive = isActive
	i.Stake = stake
	i.Rank = rank
}
