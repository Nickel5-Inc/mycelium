package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
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
// It provides methods for verifying validators through a Python service endpoint
// and maintaining the validator set.
type ValidatorRegistry struct {
	mu         sync.RWMutex
	validators map[string]ValidatorStatus
	verifyURL  string  // URL for Python verification endpoint
	minStake   float64 // Minimum stake required for validators
}

// NewValidatorRegistry creates a new validator registry instance.
// It initializes the validator tracking map and sets up the verification endpoint.
func NewValidatorRegistry(verifyURL string, minStake float64) *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: make(map[string]ValidatorStatus),
		verifyURL:  verifyURL,
		minStake:   minStake,
	}
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
	req := VerifyRequest{
		PeerID:    peerID,
		Signature: signature,
		Message:   message,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return false, err
	}

	// Call Python verification endpoint
	httpReq, err := http.NewRequestWithContext(ctx, "POST", vr.verifyURL, bytes.NewReader(jsonData))
	if err != nil {
		return false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("verification failed with status: %d", resp.StatusCode)
	}

	var verifyResp VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return false, err
	}

	if verifyResp.Valid {
		status := ValidatorStatus{
			PeerID:    peerID,
			IsActive:  true,
			LastSeen:  time.Now(),
			Stake:     verifyResp.Stake,
			Signature: signature,
		}

		// Validate the status before adding to registry
		if err := status.ValidateStatus(vr.minStake); err != nil {
			return false, fmt.Errorf("invalid validator status: %w", err)
		}

		vr.mu.Lock()
		vr.validators[peerID] = status
		vr.mu.Unlock()
	}

	return verifyResp.Valid, nil
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
