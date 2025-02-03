package identity

import (
	"time"

	"mycelium/internal/substrate"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// Identity represents a node's identity in the Mycelium network
type Identity struct {
	// Core identification
	ID      string            // Unique identifier
	Hotkey  *substrate.Wallet // Validator hotkey
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
func New(id string, hotkey *substrate.Wallet, netUID uint16) *Identity {
	return &Identity{
		ID:       id,
		Hotkey:   hotkey,
		NetUID:   netUID,
		Version:  "1.0.0", // TODO: Make configurable
		Metadata: make(map[string]string),
	}
}

// AccountID returns the identity's AccountID for chain operations
func (i *Identity) AccountID() types.AccountID {
	var id types.AccountID
	copy(id[:], []byte(i.ID))
	return id
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
