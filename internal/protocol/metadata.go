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

// ShardRange represents a range of data managed by a node
type ShardRange struct {
	ShardID   string    `json:"shard_id"`
	StartKey  string    `json:"start_key"`
	EndKey    string    `json:"end_key"`
	IsPrimary bool      `json:"is_primary"`
	ReplicaOf string    `json:"replica_of,omitempty"`
	LastSync  time.Time `json:"last_sync"`
}

// ProtocolVersion represents a specific version of the protocol
type ProtocolVersion struct {
	Version              string          `json:"version"`
	MinCompatibleVersion string          `json:"min_compatible_version"`
	Features             map[string]bool `json:"features"`
	IsActive             bool            `json:"is_active"`
	ActivationTime       time.Time       `json:"activation_time"`
}

// MetadataManager handles protocol version compatibility and feature negotiation
type MetadataManager struct {
	currentVersion *version.Version
	minVersion     *version.Version
	features       map[string]bool
	nodes          map[string]NodeMetadata
}

// NewMetadataManager creates a new metadata manager
func NewMetadataManager() (*MetadataManager, error) {
	current, err := version.NewVersion(CurrentVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid current version: %w", err)
	}

	min, err := version.NewVersion(MinCompatibleVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid min version: %w", err)
	}

	return &MetadataManager{
		currentVersion: current,
		minVersion:     min,
		features: map[string]bool{
			FeatureSharding:           true,
			FeatureReplication:        true,
			FeatureConsensus:          true,
			FeatureCompression:        true,
			FeatureEncryption:         true,
			FeatureMetadataSync:       true,
			FeatureAutoScaling:        true,
			FeatureLoadBalancing:      true,
			FeatureFailover:           true,
			FeatureLeaderElection:     true,
			FeatureConflictResolution: true,
		},
		nodes: make(map[string]NodeMetadata),
	}, nil
}

// IsCompatible checks if a given version is compatible with the current protocol
func (mm *MetadataManager) IsCompatible(nodeVersion string) (bool, error) {
	v, err := version.NewVersion(nodeVersion)
	if err != nil {
		return false, fmt.Errorf("invalid version string: %w", err)
	}

	return !v.LessThan(mm.minVersion), nil
}

// UpdateNodeMetadata updates the metadata for a node
func (mm *MetadataManager) UpdateNodeMetadata(metadata NodeMetadata) error {
	compatible, err := mm.IsCompatible(metadata.Version)
	if err != nil {
		return err
	}
	if !compatible {
		return fmt.Errorf("incompatible version: %s", metadata.Version)
	}

	metadata.LastSeen = time.Now()
	mm.nodes[metadata.NodeID] = metadata
	return nil
}

// GetActiveNodes returns a list of currently active nodes
func (mm *MetadataManager) GetActiveNodes() []NodeMetadata {
	var active []NodeMetadata
	for _, node := range mm.nodes {
		if node.IsActive && time.Since(node.LastSeen) < time.Hour {
			active = append(active, node)
		}
	}

	// Sort by node ID for consistency
	sort.Slice(active, func(i, j int) bool {
		return active[i].NodeID < active[j].NodeID
	})

	return active
}

// GetShardNodes returns nodes responsible for a given shard
func (mm *MetadataManager) GetShardNodes(shardID string) []NodeMetadata {
	var nodes []NodeMetadata
	for _, node := range mm.nodes {
		for _, shard := range node.Shards {
			if shard.ShardID == shardID {
				nodes = append(nodes, node)
				break
			}
		}
	}
	return nodes
}

// SupportsFeature checks if a specific feature is supported
func (mm *MetadataManager) SupportsFeature(feature string) bool {
	return mm.features[feature]
}

// GetFeatureMatrix returns the feature compatibility matrix for all active nodes
func (mm *MetadataManager) GetFeatureMatrix() map[string]map[string]bool {
	matrix := make(map[string]map[string]bool)
	for nodeID, node := range mm.nodes {
		if node.IsActive {
			matrix[nodeID] = node.Features
		}
	}
	return matrix
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

	// Update nodes
	for _, node := range decoded.Nodes {
		if err := mm.UpdateNodeMetadata(node); err != nil {
			return fmt.Errorf("failed to update node %s: %w", node.NodeID, err)
		}
	}

	return nil
}
