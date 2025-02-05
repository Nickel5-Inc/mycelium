package protocol

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/hashicorp/go-version"
)

// Protocol versioning constants
const (
	CurrentVersion       = "1.0.0"
	MinCompatibleVersion = "1.0.0"
)

// Feature flags for protocol capabilities
const (
	FeatureSharding           = "sharding"
	FeatureReplication        = "replication"
	FeatureConsensus          = "consensus"
	FeatureCompression        = "compression"
	FeatureEncryption         = "encryption"
	FeatureMetadataSync       = "metadata_sync"
	FeatureAutoScaling        = "auto_scaling"
	FeatureLoadBalancing      = "load_balancing"
	FeatureFailover           = "failover"
	FeatureLeaderElection     = "leader_election"
	FeatureConflictResolution = "conflict_resolution"
)

// NodeMetadata represents a node's capabilities and status
type NodeMetadata struct {
	NodeID       string            `json:"node_id"`
	Version      string            `json:"version"`
	Features     map[string]bool   `json:"features"`
	Shards       []ShardRange      `json:"shards"`
	Capabilities map[string]string `json:"capabilities"`
	LastSeen     time.Time         `json:"last_seen"`
	IsActive     bool              `json:"is_active"`
}

// ShardRange represents a range of shards managed by a node
type ShardRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// MetadataManager manages node metadata and version compatibility
type MetadataManager struct {
	nodeID   string
	version  string
	features map[string]bool
	nodes    map[string]NodeMetadata
}

// NewMetadataManager creates a new metadata manager
func NewMetadataManager(nodeID, version string) *MetadataManager {
	return &MetadataManager{
		nodeID:   nodeID,
		version:  version,
		features: make(map[string]bool),
		nodes:    make(map[string]NodeMetadata),
	}
}

// IsCompatible checks if the given version is compatible
func (mm *MetadataManager) IsCompatible(otherVersion string) (bool, error) {
	current, err := version.NewVersion(CurrentVersion)
	if err != nil {
		return false, fmt.Errorf("invalid current version: %w", err)
	}

	min, err := version.NewVersion(MinCompatibleVersion)
	if err != nil {
		return false, fmt.Errorf("invalid min version: %w", err)
	}

	other, err := version.NewVersion(otherVersion)
	if err != nil {
		return false, fmt.Errorf("invalid version to check: %w", err)
	}

	// Check if version is between min and current
	return !other.LessThan(min) && !other.GreaterThan(current), nil
}

// GetActiveNodes returns a list of active nodes sorted by node ID
func (mm *MetadataManager) GetActiveNodes() []NodeMetadata {
	active := make([]NodeMetadata, 0)
	for _, node := range mm.nodes {
		if node.IsActive {
			active = append(active, node)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].NodeID < active[j].NodeID
	})
	return active
}

// EncodeMetadata encodes node metadata for transmission
func (mm *MetadataManager) EncodeMetadata() ([]byte, error) {
	return json.Marshal(struct {
		Version    string          `json:"version"`
		MinVersion string          `json:"min_version"`
		Features   map[string]bool `json:"features"`
		Nodes      []NodeMetadata  `json:"nodes"`
	}{
		Version:    CurrentVersion,
		MinVersion: MinCompatibleVersion,
		Features:   mm.features,
		Nodes:      mm.GetActiveNodes(),
	})
}

// DecodeMetadata decodes received node metadata
func (mm *MetadataManager) DecodeMetadata(data []byte) error {
	var decoded struct {
		Version    string          `json:"version"`
		MinVersion string          `json:"min_version"`
		Features   map[string]bool `json:"features"`
		Nodes      []NodeMetadata  `json:"nodes"`
	}

	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("failed to decode metadata: %w", err)
	}

	// Check version compatibility
	compatible, err := mm.IsCompatible(decoded.Version)
	if err != nil {
		return err
	}
	if !compatible {
		return fmt.Errorf("incompatible version: %s", decoded.Version)
	}

	// Update features and nodes
	mm.features = decoded.Features
	for _, node := range decoded.Nodes {
		mm.nodes[node.NodeID] = node
	}

	return nil
}
