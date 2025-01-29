package peer

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"mycelium/internal/chain"
)

// ValidatorStatus represents a validator's current state in the network.
// It tracks the validator's activity, stake, and authentication details.
type ValidatorStatus struct {
	PeerID    string            `json:"peer_id"`   // Unique identifier for the validator
	NetUID    uint16            `json:"net_uid"`   // Bittensor subnet UID
	UID       uint16            `json:"uid"`       // Validator's UID on the subnet
	Hotkey    string            `json:"hotkey"`    // SS58 hotkey address
	Coldkey   string            `json:"coldkey"`   // SS58 coldkey address
	Stake     float64           `json:"stake"`     // Total TAO staked
	IP        string            `json:"ip"`        // Validator's IP address
	Port      uint16            `json:"port"`      // Validator's primary port
	IsActive  bool              `json:"is_active"` // Whether the validator is currently active
	LastSeen  time.Time         `json:"last_seen"` // Last time the validator was seen
	Signature []byte            `json:"signature"` // Last known signature from the validator
	Metadata  map[string]string `json:"metadata"`  // Additional validator metadata
}

// ValidatorRegistry manages the list of valid validators and their current status.
type ValidatorRegistry struct {
	mu         sync.RWMutex
	validators map[string]ValidatorStatus
	verifier   *chain.SubstrateVerifier
	minStake   float64
}

// NewValidatorRegistry creates a new validator registry instance.
func NewValidatorRegistry(wsURL string, minStake float64) (*ValidatorRegistry, error) {
	if err := chain.InitPythonBridge(wsURL); err != nil {
		return nil, fmt.Errorf("failed to initialize substrate bridge: %w", err)
	}

	verifier := chain.NewSubstrateVerifier(1) // TODO: Make netUID configurable
	return &ValidatorRegistry{
		validators: make(map[string]ValidatorStatus),
		verifier:   verifier,
		minStake:   minStake,
	}, nil
}

// VerifyRequest represents a verification request sent to the Python service.
type VerifyRequest struct {
	PeerID    string `json:"peer_id"`   // ID of the peer to verify
	Signature []byte `json:"signature"` // Signature to verify
	Message   []byte `json:"message"`   // Original message that was signed
}

// VerifyResponse represents the response from the Python verification service.
type VerifyResponse struct {
	Valid bool    `json:"valid"` // Whether the signature is valid
	Stake float64 `json:"stake"` // The validator's stake if valid
}

// ValidateStatus checks if a validator's status is valid
func (vs *ValidatorStatus) ValidateStatus(minStake float64) error {
	// Network validation
	if vs.IP != "" {
		if ip := net.ParseIP(vs.IP); ip == nil {
			return fmt.Errorf("invalid IP address: %s", vs.IP)
		}
	}

	if vs.Port < MinPort || vs.Port > MaxPort {
		return fmt.Errorf("port must be between %d and %d", MinPort, MaxPort)
	}

	// Identity validation
	if vs.Hotkey == "" {
		return fmt.Errorf("hotkey is required")
	}
	if !isValidSS58Address(vs.Hotkey) {
		return fmt.Errorf("invalid hotkey format")
	}

	if vs.Coldkey == "" {
		return fmt.Errorf("coldkey is required")
	}
	if !isValidSS58Address(vs.Coldkey) {
		return fmt.Errorf("invalid coldkey format")
	}

	// Stake validation
	if vs.Stake < 0 {
		return fmt.Errorf("stake cannot be negative")
	}
	if vs.Stake < minStake {
		return fmt.Errorf("stake (%f) is below minimum requirement (%f)", vs.Stake, minStake)
	}

	// Signature validation
	if len(vs.Signature) == 0 {
		return fmt.Errorf("signature is required")
	}

	return nil
}

// Constants for validation
const (
	MinPort    = 1024
	MaxPort    = 65535
	SS58Prefix = "5" // Substrate/Polkadot SS58 prefix
)

// isValidSS58Address checks if the given string is a valid SS58 address
func isValidSS58Address(addr string) bool {
	if !strings.HasPrefix(addr, SS58Prefix) {
		return false
	}
	return len(addr) == 48 // Standard SS58 address length
}

// VerifyValidator checks if a peer is a valid validator
func (vr *ValidatorRegistry) VerifyValidator(ctx context.Context, peerID string, signature, message []byte) (bool, error) {
	// First check if we have cached validator status
	vr.mu.RLock()
	status, exists := vr.validators[peerID]
	vr.mu.RUnlock()

	if exists && time.Since(status.LastSeen) < time.Hour {
		return status.IsActive && status.Stake >= vr.minStake, nil
	}

	// Verify signature
	valid, err := chain.VerifySignature(peerID, string(signature), string(message))
	if err != nil || !valid {
		return false, fmt.Errorf("signature verification failed: %w", err)
	}

	// Get stake amount
	stake, err := chain.GetStake(peerID, status.Coldkey)
	if err != nil {
		return false, fmt.Errorf("failed to get stake: %w", err)
	}

	// Update validator status
	vr.mu.Lock()
	vr.validators[peerID] = ValidatorStatus{
		PeerID:    peerID,
		Stake:     stake,
		IsActive:  true,
		LastSeen:  time.Now(),
		Signature: signature,
	}
	vr.mu.Unlock()

	return stake >= vr.minStake, nil
}

// IsValidator checks if a peer is currently a known and active validator.
// A validator is considered inactive if they haven't been seen in the last hour.
func (vr *ValidatorRegistry) IsValidator(peerID string) bool {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	status, exists := vr.validators[peerID]
	if !exists {
		return false
	}

	// Consider validators inactive after 1 hour without updates
	if time.Since(status.LastSeen) > time.Hour {
		return false
	}

	return status.IsActive
}

// GetValidators returns a list of all currently active validators.
// Validators that haven't been seen in the last hour are excluded.
func (vr *ValidatorRegistry) GetValidators() []ValidatorStatus {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	var active []ValidatorStatus
	for _, status := range vr.validators {
		if status.IsActive && time.Since(status.LastSeen) <= time.Hour {
			active = append(active, status)
		}
	}
	return active
}

// RemoveValidator removes a validator from the registry.
// This is typically called when a validator becomes inactive or invalid.
func (vr *ValidatorRegistry) RemoveValidator(peerID string) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	delete(vr.validators, peerID)
}

// CleanupInactive removes validators that haven't been seen in the last hour.
// This helps maintain an up-to-date set of active validators.
func (vr *ValidatorRegistry) CleanupInactive() {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	for peerID, status := range vr.validators {
		if time.Since(status.LastSeen) > time.Hour {
			delete(vr.validators, peerID)
		}
	}
}

// GetValidator returns the validator status for a given peer ID
func (vr *ValidatorRegistry) GetValidator(peerID string) (ValidatorStatus, bool) {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	status, exists := vr.validators[peerID]
	return status, exists
}

// Close cleans up resources
func (vr *ValidatorRegistry) Close() error {
	chain.Cleanup()
	return nil
}
