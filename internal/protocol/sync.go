package protocol

import (
	"time"
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

// SyncUpdate represents a blacklist synchronization update
type SyncUpdate struct {
	Greylist  map[string]IPStatus `json:"greylist"`
	Blacklist map[string]IPStatus `json:"blacklist"`
}
