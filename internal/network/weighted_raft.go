package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/jackc/pgx/v5"
)

// WeightedVoter represents a voter with associated weight factors
type WeightedVoter struct {
	// Basic info
	NodeID     string
	Tier       int
	PublicKey  string
	JoinedAt   time.Time
	LastActive time.Time

	// Weight factors
	StakeAmount uint64
	Uptime      float64
	VouchCount  int
	Performance float64

	// Authorization
	AuthorizedBy []string // NodeIDs of authorizing Tier 0 validators
	Vouchers     []string // NodeIDs of vouching validators

	// Additional metadata
	Metadata map[string]interface{}
}

// WeightedRaft extends Raft with weighted voting
type WeightedRaft struct {
	mu sync.RWMutex

	// Underlying Raft instance
	*raft.Raft

	// Voter registry
	voters map[string]*WeightedVoter

	// Configuration
	config WeightedRaftConfig

	// Database connection
	db Database
}

// WeightedRaftConfig holds configuration for weighted voting
type WeightedRaftConfig struct {
	// Minimum weights required for leadership
	MinTier0Weight float64
	MinTier1Weight float64
	MinTotalWeight float64

	// Time requirements
	MinUptimeForTier1 time.Duration
	MinUptimeForTier2 time.Duration
	ProbationPeriod   time.Duration

	// Stake requirements
	MinStakeForTier1 uint64
	MinStakeForTier2 uint64

	// Authorization requirements
	MinTier0Signatures int
	MinTier1Vouches    int
}

// Database interface for emergency log persistence
type Database interface {
	WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error
}

// NewWeightedRaft creates a new weighted Raft instance
func NewWeightedRaft(config WeightedRaftConfig, raftConfig *raft.Config, db Database) (*WeightedRaft, error) {
	r := &WeightedRaft{
		voters: make(map[string]*WeightedVoter),
		config: config,
		db:     db,
	}

	// Wrap the vote handler
	originalHandler := raftConfig.NotifyCh
	notifyCh := make(chan bool, 1)
	raftConfig.NotifyCh = notifyCh
	go r.handleVotes(notifyCh, originalHandler)

	return r, nil
}

// handleVotes processes votes with weights
func (wr *WeightedRaft) handleVotes(voteCh <-chan bool, resultCh chan<- bool) {
	for vote := range voteCh {
		if vote {
			// Calculate weighted votes
			tier0Weight := float64(0)
			tier1Weight := float64(0)
			totalWeight := float64(0)

			wr.mu.RLock()
			for _, voter := range wr.voters {
				weight := wr.calculateVoterWeight(voter)

				switch voter.Tier {
				case 0:
					tier0Weight += weight
				case 1:
					tier1Weight += weight
				}
				totalWeight += weight
			}
			wr.mu.RUnlock()

			// Check if weights meet requirements
			if tier0Weight >= wr.config.MinTier0Weight &&
				tier1Weight >= wr.config.MinTier1Weight &&
				totalWeight >= wr.config.MinTotalWeight {
				resultCh <- true
			} else {
				resultCh <- false
			}
		} else {
			resultCh <- false
		}
	}
}

// calculateVoterWeight determines a voter's voting weight
func (wr *WeightedRaft) calculateVoterWeight(voter *WeightedVoter) float64 {
	// Base weight by tier
	baseWeight := float64(0)
	switch voter.Tier {
	case 0:
		baseWeight = 100
	case 1:
		baseWeight = 50
	case 2:
		baseWeight = 25
	}

	// Uptime factor (0.5 - 1.0)
	uptimeFactor := 0.5 + (voter.Uptime * 0.5)

	// Stake factor (0.0 - 1.0)
	var stakeFactor float64
	switch voter.Tier {
	case 1:
		stakeFactor = float64(voter.StakeAmount) / float64(wr.config.MinStakeForTier1)
	case 2:
		stakeFactor = float64(voter.StakeAmount) / float64(wr.config.MinStakeForTier2)
	default:
		stakeFactor = 1.0
	}
	if stakeFactor > 1.0 {
		stakeFactor = 1.0
	}

	// Performance factor (0.5 - 1.0)
	perfFactor := 0.5 + (voter.Performance * 0.5)

	// Combine factors
	return baseWeight * uptimeFactor * stakeFactor * perfFactor
}

// RegisterVoter adds or updates a voter
func (wr *WeightedRaft) RegisterVoter(voter *WeightedVoter) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	// Validate tier requirements
	switch voter.Tier {
	case 0:
		// Tier 0 must be explicitly authorized in initial config
		return fmt.Errorf("tier 0 validators must be configured at startup")

	case 1:
		if err := wr.validateTier1(voter); err != nil {
			return err
		}

	case 2:
		if err := wr.validateTier2(voter); err != nil {
			return err
		}
	}

	wr.voters[voter.NodeID] = voter
	return nil
}

// validateTier1 checks Tier 1 validator requirements
func (wr *WeightedRaft) validateTier1(voter *WeightedVoter) error {
	// Check uptime
	if time.Since(voter.JoinedAt) < wr.config.MinUptimeForTier1 {
		return fmt.Errorf("insufficient uptime for tier 1")
	}

	// Check stake
	if voter.StakeAmount < wr.config.MinStakeForTier1 {
		return fmt.Errorf("insufficient stake for tier 1")
	}

	// Check Tier 0 signatures
	tier0Sigs := 0
	for _, authorizer := range voter.AuthorizedBy {
		if auth, exists := wr.voters[authorizer]; exists && auth.Tier == 0 {
			tier0Sigs++
		}
	}
	if tier0Sigs < wr.config.MinTier0Signatures {
		return fmt.Errorf("insufficient tier 0 signatures")
	}

	return nil
}

// validateTier2 checks Tier 2 validator requirements
func (wr *WeightedRaft) validateTier2(voter *WeightedVoter) error {
	// Check uptime
	if time.Since(voter.JoinedAt) < wr.config.MinUptimeForTier2 {
		return fmt.Errorf("insufficient uptime for tier 2")
	}

	// Check stake
	if voter.StakeAmount < wr.config.MinStakeForTier2 {
		return fmt.Errorf("insufficient stake for tier 2")
	}

	// Check vouches from Tier 1
	tier1Vouches := 0
	for _, voucher := range voter.Vouchers {
		if v, exists := wr.voters[voucher]; exists && v.Tier == 1 {
			tier1Vouches++
		}
	}
	if tier1Vouches < wr.config.MinTier1Vouches {
		return fmt.Errorf("insufficient tier 1 vouches")
	}

	return nil
}

// UpdateVoterMetrics updates a voter's performance metrics
func (wr *WeightedRaft) UpdateVoterMetrics(nodeID string, uptime float64, performance float64) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	voter, exists := wr.voters[nodeID]
	if !exists {
		return fmt.Errorf("voter not found: %s", nodeID)
	}

	voter.Uptime = uptime
	voter.Performance = performance
	voter.LastActive = time.Now()

	return nil
}

// EmergencyAction represents the type of emergency action
type EmergencyAction string

const (
	// ForceLeaderStep forces a leader to step down
	ForceLeaderStep EmergencyAction = "force_step_down"
	// BlockValidator prevents a validator from being elected
	BlockValidator EmergencyAction = "block_validator"
	// UnblockValidator re-enables a validator for election
	UnblockValidator EmergencyAction = "unblock_validator"
	// ForceSyncState forces a state sync from a specific node
	ForceSyncState EmergencyAction = "force_sync"
)

// EmergencyActionLog represents a logged emergency action
type EmergencyActionLog struct {
	Timestamp  time.Time
	Initiator  string
	Target     string
	Action     EmergencyAction
	RaftState  raft.RaftState
	Signatures map[string][]byte
}

// EmergencyOverride allows Tier 0 to force leadership changes
func (wr *WeightedRaft) EmergencyOverride(initiatorID string, targetID string, action EmergencyAction) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	// Verify initiator is Tier 0
	initiator, exists := wr.voters[initiatorID]
	if !exists || initiator.Tier != 0 {
		return fmt.Errorf("emergency override can only be initiated by Tier 0 validators")
	}

	// Verify target exists
	target, exists := wr.voters[targetID]
	if !exists {
		return fmt.Errorf("target validator %s not found", targetID)
	}

	switch action {
	case ForceLeaderStep:
		if wr.State() != raft.Leader || string(wr.Leader()) != targetID {
			return fmt.Errorf("target is not the current leader")
		}
		// Force step down by updating voter weight to 0
		target.Performance = 0
		target.Uptime = 0
		// Trigger immediate re-election
		future := wr.Raft.LeadershipTransfer()
		return future.Error()

	case BlockValidator:
		// Mark validator as inactive and zero out their voting weight
		target.Performance = 0
		target.Uptime = 0
		if target.Metadata == nil {
			target.Metadata = make(map[string]interface{})
		}
		// Add to blocked list
		target.Metadata["blocked"] = true
		target.Metadata["blocked_by"] = initiatorID
		target.Metadata["blocked_at"] = time.Now()
		target.Metadata["block_reason"] = "emergency override"

		// If they're the leader, force step down
		if wr.State() == raft.Leader && string(wr.Leader()) == targetID {
			future := wr.Raft.LeadershipTransfer()
			return future.Error()
		}

	case UnblockValidator:
		if target.Metadata == nil {
			target.Metadata = make(map[string]interface{})
		}
		// Remove blocked status
		delete(target.Metadata, "blocked")
		// Reset metrics to probation levels
		target.Performance = 0.5
		target.Uptime = 0.5

	case ForceSyncState:
		if target.Tier == 0 {
			// Get state from another Tier 0 validator
			future := wr.Raft.Snapshot()
			return future.Error()
		}
		return fmt.Errorf("force sync only allowed for Tier 0 validators")

	default:
		return fmt.Errorf("unknown emergency action: %s", action)
	}

	// Log the emergency action
	return wr.logEmergencyAction(initiatorID, targetID, action)
}

// logEmergencyAction records emergency actions for audit
func (wr *WeightedRaft) logEmergencyAction(initiator, target string, action EmergencyAction) error {
	log := EmergencyActionLog{
		Timestamp:  time.Now(),
		Initiator:  initiator,
		Target:     target,
		Action:     action,
		RaftState:  wr.State(),
		Signatures: make(map[string][]byte),
	}

	// Collect signatures from other Tier 0 validators
	for id, voter := range wr.voters {
		if voter.Tier == 0 {
			// In practice, you'd implement actual signing here
			log.Signatures[id] = []byte("signature")
		}
	}

	// Persist to database
	return wr.db.WithTx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO emergency_action_logs 
			(timestamp, initiator_id, target_id, action, raft_state, signatures)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			log.Timestamp,
			log.Initiator,
			log.Target,
			log.Action,
			log.RaftState,
			log.Signatures,
		)
		return err
	})
}
