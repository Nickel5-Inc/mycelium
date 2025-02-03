package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/raft"
)

// WorkItem represents a task to be processed by miners
type WorkItem struct {
	ID        string
	ShardID   string
	Type      string
	Data      []byte
	Deadline  time.Time
	MinScores int // Minimum number of validator scores needed
}

// WorkScore represents a validator's score for completed work
type WorkScore struct {
	WorkID      string
	ValidatorID string
	Score       float64
	Confidence  float64
	Timestamp   time.Time
}

// ConsensusManager coordinates work distribution and scoring
type ConsensusManager struct {
	mu sync.RWMutex
	// Raft nodes for each shard
	shardRafts map[string]*raft.Raft
	// Active work items
	activeWork map[string]*WorkItem
	// Work scores from validators
	workScores map[string][]WorkScore
	// Minimum consensus threshold (percentage of validators)
	consensusThreshold float64
	// Connection manager for validator communication
	connMgr *ConnectionManager
	// Pub/sub manager for miner communication
	pubsub *PubSubManager
}

// NewConsensusManager creates a new consensus manager
func NewConsensusManager(connMgr *ConnectionManager, pubsub *PubSubManager, threshold float64) *ConsensusManager {
	return &ConsensusManager{
		shardRafts:         make(map[string]*raft.Raft),
		activeWork:         make(map[string]*WorkItem),
		workScores:         make(map[string][]WorkScore),
		consensusThreshold: threshold,
		connMgr:            connMgr,
		pubsub:             pubsub,
	}
}

// DistributeWork sends work to miners and collects validator scores
func (cm *ConsensusManager) DistributeWork(ctx context.Context, work *WorkItem) error {
	cm.mu.Lock()
	cm.activeWork[work.ID] = work
	cm.mu.Unlock()

	// Publish work to miners via NATS
	update := MinerUpdate{
		ShardID: work.ShardID,
		Type:    "work_request",
		Data:    work.Data,
	}

	if err := cm.pubsub.PublishUpdate(ctx, update); err != nil {
		return fmt.Errorf("failed to publish work: %w", err)
	}

	return nil
}

// SubmitScore allows validators to score completed work
func (cm *ConsensusManager) SubmitScore(score WorkScore) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Verify work is active
	work, exists := cm.activeWork[score.WorkID]
	if !exists {
		return fmt.Errorf("work %s not found", score.WorkID)
	}

	// Add score
	cm.workScores[score.WorkID] = append(cm.workScores[score.WorkID], score)

	// Check if we have enough scores
	scores := cm.workScores[score.WorkID]
	if len(scores) >= work.MinScores {
		// Calculate consensus
		consensus, result := cm.calculateConsensus(scores)
		if consensus >= cm.consensusThreshold {
			// Commit result to database via Raft
			return cm.commitResult(work.ShardID, work.ID, result)
		}
	}

	return nil
}

// calculateConsensus determines if validators agree on work quality
func (cm *ConsensusManager) calculateConsensus(scores []WorkScore) (float64, float64) {
	if len(scores) == 0 {
		return 0, 0
	}

	// Weight scores by validator confidence
	var weightedSum, weightSum float64
	for _, score := range scores {
		weightedSum += score.Score * score.Confidence
		weightSum += score.Confidence
	}

	// Calculate weighted average
	avgScore := weightedSum / weightSum

	// Calculate agreement percentage
	var agreementCount float64
	for _, score := range scores {
		// Consider scores within 10% of average as "agreeing"
		if abs(score.Score-avgScore) <= (avgScore * 0.1) {
			agreementCount++
		}
	}

	return agreementCount / float64(len(scores)), avgScore
}

// commitResult commits consensus result to the database
func (cm *ConsensusManager) commitResult(shardID, workID string, result float64) error {
	raftNode, exists := cm.shardRafts[shardID]
	if !exists {
		return fmt.Errorf("no raft node for shard %s", shardID)
	}

	// Create log entry
	entry := ConsensusEntry{
		WorkID: workID,
		Result: result,
		Time:   time.Now(),
	}

	// Apply via Raft to ensure consistency
	data, err := entry.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	future := raftNode.Apply(data, 5*time.Second)
	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to apply raft entry: %w", err)
	}

	return nil
}

// abs returns the absolute value of x
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ConsensusEntry represents a log entry for Raft consensus
type ConsensusEntry struct {
	WorkID string
	Result float64
	Time   time.Time
}

// Marshal serializes the consensus entry
func (e *ConsensusEntry) Marshal() ([]byte, error) {
	// Implementation depends on chosen serialization format
	// Could use protobuf, msgpack, etc.
	return nil, nil
}
