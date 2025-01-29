package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ValidatorStatus represents a validator's current state in the network.
// It tracks the validator's activity, stake, and authentication details.
type ValidatorStatus struct {
	PeerID    string    `json:"peer_id"`   // Unique identifier for the validator
	IsActive  bool      `json:"is_active"` // Whether the validator is currently active
	LastSeen  time.Time `json:"last_seen"` // Last time the validator was seen
	Stake     float64   `json:"stake"`     // Validator's stake in the network
	Signature []byte    `json:"signature"` // Last known signature from the validator
}

// ValidatorRegistry manages the list of valid validators and their current status.
// It provides methods for verifying validators through a Python service endpoint
// and maintaining the validator set.
type ValidatorRegistry struct {
	mu         sync.RWMutex
	validators map[string]ValidatorStatus
	verifyURL  string // URL for Python verification endpoint
}

// NewValidatorRegistry creates a new validator registry instance.
// It initializes the validator tracking map and sets up the verification endpoint.
func NewValidatorRegistry(verifyURL string) *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: make(map[string]ValidatorStatus),
		verifyURL:  verifyURL,
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

// VerifyValidator checks if a peer is a valid validator by verifying their signature
// through the Python verification service. If valid, the validator is added to the registry.
// Returns true if the validator is valid, false otherwise.
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
		vr.mu.Lock()
		vr.validators[peerID] = ValidatorStatus{
			PeerID:    peerID,
			IsActive:  true,
			LastSeen:  time.Now(),
			Stake:     verifyResp.Stake,
			Signature: signature,
		}
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
