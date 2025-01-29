package peer

import (
	"sync"
	"time"
)

const (
	// Default thresholds for blacklisting
	DefaultMaxFailedAttempts  = 5 // Number of failed attempts before greylist
	DefaultGreylistDuration   = 1 * time.Hour
	DefaultBlacklistThreshold = 10     // Number of greylist violations before blacklist
	DefaultMinStakeToUnblock  = 1000.0 // Minimum stake required to be removed from blacklist
)

// IPStatus represents the current status of an IP address
type IPStatus struct {
	IP             string    `json:"ip"`
	FailedAttempts int       `json:"failed_attempts"`
	FirstFailure   time.Time `json:"first_failure"`
	LastFailure    time.Time `json:"last_failure"`
	GreylistCount  int       `json:"greylist_count"`   // Number of times added to greylist
	BlacklistedAt  time.Time `json:"blacklisted_at"`   // When the IP was blacklisted
	LastStakeCheck float64   `json:"last_stake_check"` // Last known stake amount
}

// BlacklistManager handles IP blacklisting and synchronization across the network
type BlacklistManager struct {
	mu         sync.RWMutex
	greylist   map[string]IPStatus // IP -> Status mapping for greylist
	blacklist  map[string]IPStatus // IP -> Status mapping for blacklist
	validators *ValidatorRegistry  // Reference to validator registry for stake checks

	// Configuration
	maxFailedAttempts  int
	greylistDuration   time.Duration
	blacklistThreshold int
	minStakeToUnblock  float64
}

// NewBlacklistManager creates a new blacklist manager instance
func NewBlacklistManager(validators *ValidatorRegistry) *BlacklistManager {
	return &BlacklistManager{
		greylist:           make(map[string]IPStatus),
		blacklist:          make(map[string]IPStatus),
		validators:         validators,
		maxFailedAttempts:  DefaultMaxFailedAttempts,
		greylistDuration:   DefaultGreylistDuration,
		blacklistThreshold: DefaultBlacklistThreshold,
		minStakeToUnblock:  DefaultMinStakeToUnblock,
	}
}

// RecordFailedAttempt records a failed connection attempt from an IP
func (bm *BlacklistManager) RecordFailedAttempt(ip string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Check if IP is already blacklisted
	if _, blacklisted := bm.blacklist[ip]; blacklisted {
		return
	}

	now := time.Now()
	status, exists := bm.greylist[ip]
	if !exists {
		status = IPStatus{
			IP:           ip,
			FirstFailure: now,
		}
	}

	status.FailedAttempts++
	status.LastFailure = now

	// Check if should be added to greylist
	if status.FailedAttempts >= bm.maxFailedAttempts {
		status.GreylistCount++
		// Check if should be blacklisted
		if status.GreylistCount >= bm.blacklistThreshold {
			status.BlacklistedAt = now
			bm.blacklist[ip] = status
			delete(bm.greylist, ip)
			return
		}
		// Reset failed attempts counter when added to greylist
		status.FailedAttempts = 0
	}

	bm.greylist[ip] = status
}

// IsBlocked checks if an IP is currently blocked (either greylisted or blacklisted)
func (bm *BlacklistManager) IsBlocked(ip string) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	// Check blacklist first
	if _, blacklisted := bm.blacklist[ip]; blacklisted {
		return true
	}

	// Check greylist
	if status, greylisted := bm.greylist[ip]; greylisted {
		// Remove from greylist if duration has expired
		if time.Since(status.LastFailure) > bm.greylistDuration {
			bm.mu.RUnlock()
			bm.mu.Lock()
			delete(bm.greylist, ip)
			bm.mu.Unlock()
			return false
		}
		return true
	}

	return false
}

// CheckStakeAndUnblock checks if an IP has sufficient stake to be unblocked
func (bm *BlacklistManager) CheckStakeAndUnblock(ip string) bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	status, blacklisted := bm.blacklist[ip]
	if !blacklisted {
		return false
	}

	// Find total stake for this IP
	totalStake := 0.0
	for _, validator := range bm.validators.GetValidators() {
		if validator.IP == ip {
			totalStake += validator.Stake
		}
	}

	status.LastStakeCheck = totalStake
	if totalStake >= bm.minStakeToUnblock {
		delete(bm.blacklist, ip)
		return true
	}

	bm.blacklist[ip] = status
	return false
}

// SyncUpdate represents a blacklist sync message
type SyncUpdate struct {
	Greylist  map[string]IPStatus `json:"greylist"`
	Blacklist map[string]IPStatus `json:"blacklist"`
}

// GetSyncUpdate returns the current state for mesh synchronization
func (bm *BlacklistManager) GetSyncUpdate() *SyncUpdate {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return &SyncUpdate{
		Greylist:  copyStatusMap(bm.greylist),
		Blacklist: copyStatusMap(bm.blacklist),
	}
}

// ApplySyncUpdate merges an update from another node
func (bm *BlacklistManager) ApplySyncUpdate(update *SyncUpdate) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Merge greylist
	for ip, status := range update.Greylist {
		if existing, ok := bm.greylist[ip]; ok {
			// Keep the more severe status
			if status.GreylistCount > existing.GreylistCount {
				bm.greylist[ip] = status
			}
		} else {
			bm.greylist[ip] = status
		}
	}

	// Merge blacklist
	for ip, status := range update.Blacklist {
		if existing, ok := bm.blacklist[ip]; ok {
			// Keep the more recent blacklist entry
			if status.BlacklistedAt.After(existing.BlacklistedAt) {
				bm.blacklist[ip] = status
			}
		} else {
			bm.blacklist[ip] = status
		}
	}
}

// Helper function to deep copy a status map
func copyStatusMap(m map[string]IPStatus) map[string]IPStatus {
	result := make(map[string]IPStatus, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// Cleanup removes expired entries from the greylist
func (bm *BlacklistManager) Cleanup() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	now := time.Now()
	for ip, status := range bm.greylist {
		if now.Sub(status.LastFailure) > bm.greylistDuration {
			delete(bm.greylist, ip)
		}
	}
}
