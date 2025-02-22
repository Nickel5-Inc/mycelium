use std::collections::{HashMap, HashSet};
use std::net::SocketAddr;
use std::sync::Arc;
use serde::{Serialize, Deserialize};
use tokio::sync::RwLock;

use crate::error::{Error, Result};
use super::{NodeRole, NodeStatus};
use super::metrics::MetricsCollector;
use super::sql::{SqlAnalyzer, TableOperation};

/// Replication policy for a table
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TablePolicy {
    /// Table name
    pub table: String,
    /// Replication factor (percentage of nodes that should have this table)
    pub replication_factor: f32,
    /// Whether this table requires validator nodes only
    pub validator_only: bool,
    /// Minimum number of validator copies
    pub min_validator_copies: u32,
    /// Read access roles
    pub read_roles: HashSet<NodeRole>,
    /// Write access roles
    pub write_roles: HashSet<NodeRole>,
    /// Shard key columns
    pub shard_keys: Vec<String>,
    /// Whether to enable range-based sharding
    pub range_sharding: bool,
    /// Custom sharding function (if any)
    pub shard_function: Option<String>,
}

impl Default for TablePolicy {
    fn default() -> Self {
        Self {
            table: String::new(),
            replication_factor: 1.0, // Full replication
            validator_only: true,
            min_validator_copies: 2,
            read_roles: HashSet::from([NodeRole::Primary, NodeRole::Secondary]),
            write_roles: HashSet::from([NodeRole::Primary]),
            shard_keys: Vec::new(),
            range_sharding: false,
            shard_function: None,
        }
    }
}

/// Shard assignment for a table
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ShardAssignment {
    /// Table name
    pub table: String,
    /// Shard ID
    pub shard_id: u32,
    /// Shard key range start (inclusive)
    pub range_start: Option<String>,
    /// Shard key range end (exclusive)
    pub range_end: Option<String>,
    /// Primary nodes for this shard
    pub primaries: HashSet<SocketAddr>,
    /// Secondary nodes for this shard
    pub secondaries: HashSet<SocketAddr>,
}

/// Node shard information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NodeShardInfo {
    /// Node address
    pub addr: SocketAddr,
    /// Node role
    pub role: NodeRole,
    /// Node status
    pub status: NodeStatus,
    /// Available disk space
    pub disk_space: u64,
    /// Current load
    pub load: f32,
    /// Assigned shards
    pub shards: HashSet<u32>,
    /// Is validator
    pub is_validator: bool,
}

/// Sharding configuration
#[derive(Debug, Clone)]
pub struct ShardConfig {
    /// Default replication factor
    pub default_replication_factor: f32,
    /// Minimum number of validator copies
    pub min_validator_copies: u32,
    /// Maximum shard size
    pub max_shard_size: u64,
    /// Rebalance interval
    pub rebalance_interval: std::time::Duration,
    /// Load threshold for rebalancing
    pub load_threshold: f32,
}

impl Default for ShardConfig {
    fn default() -> Self {
        Self {
            default_replication_factor: 0.5, // 50% replication by default
            min_validator_copies: 2,
            max_shard_size: 1024 * 1024 * 1024, // 1GB
            rebalance_interval: std::time::Duration::from_secs(3600),
            load_threshold: 0.8,
        }
    }
}

/// Shard manager handles table sharding and replication
pub struct ShardManager {
    /// Configuration
    config: ShardConfig,
    /// Table policies
    policies: Arc<RwLock<HashMap<String, TablePolicy>>>,
    /// Shard assignments
    assignments: Arc<RwLock<HashMap<String, Vec<ShardAssignment>>>>,
    /// Node information
    nodes: Arc<RwLock<HashMap<SocketAddr, NodeShardInfo>>>,
    /// SQL analyzer
    analyzer: SqlAnalyzer,
    /// Metrics collector
    metrics: Arc<MetricsCollector>,
}

impl ShardManager {
    /// Create a new shard manager
    pub fn new(config: ShardConfig, metrics: Arc<MetricsCollector>) -> Self {
        Self {
            config,
            policies: Arc::new(RwLock::new(HashMap::new())),
            assignments: Arc::new(RwLock::new(HashMap::new())),
            nodes: Arc::new(RwLock::new(HashMap::new())),
            analyzer: SqlAnalyzer::new(super::DatabaseType::PostgreSQL),
            metrics,
        }
    }

    /// Set policy for a table
    pub async fn set_policy(&self, policy: TablePolicy) -> Result<()> {
        let mut policies = self.policies.write().await;
        policies.insert(policy.table.clone(), policy);
        self.rebalance_shards().await?;
        Ok(())
    }

    /// Get policy for a table
    pub async fn get_policy(&self, table: &str) -> Option<TablePolicy> {
        self.policies.read().await.get(table).cloned()
    }

    /// Update node information
    pub async fn update_node(&self, info: NodeShardInfo) -> Result<()> {
        let mut nodes = self.nodes.write().await;
        nodes.insert(info.addr, info);
        Ok(())
    }

    /// Check if a node can access a table
    pub async fn check_access(&self, node: &NodeShardInfo, table: &str, write: bool) -> bool {
        if let Some(policy) = self.get_policy(table).await {
            let roles = if write {
                &policy.write_roles
            } else {
                &policy.read_roles
            };
            
            // Check role-based access
            if !roles.contains(&node.role) {
                return false;
            }
            
            // Check validator requirement
            if policy.validator_only && !node.is_validator {
                return false;
            }
            
            true
        } else {
            false
        }
    }

    /// Get shard for a query
    pub async fn get_shard(&self, table: &str, query: &str) -> Result<Option<ShardAssignment>> {
        let policy = if let Some(p) = self.get_policy(table).await {
            p
        } else {
            return Ok(None);
        };

        // If no shard keys, table is not sharded
        if policy.shard_keys.is_empty() {
            return Ok(None);
        }

        // Analyze query to extract shard key values
        let operations = self.analyzer.analyze(query)?;
        for op in operations {
            if op.table == table {
                // Find shard key values in conditions
                for condition in op.conditions {
                    if policy.shard_keys.contains(&condition.column) {
                        // Find matching shard
                        let assignments = self.assignments.read().await;
                        if let Some(shards) = assignments.get(table) {
                            for shard in shards {
                                if let (Some(start), Some(end)) = (&shard.range_start, &shard.range_end) {
                                    if condition.value >= start && condition.value < end {
                                        return Ok(Some(shard.clone()));
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        Ok(None)
    }

    /// Rebalance shards across nodes
    pub async fn rebalance_shards(&self) -> Result<()> {
        let policies = self.policies.read().await;
        let mut assignments = self.assignments.write().await;
        let nodes = self.nodes.read().await;

        // Get validator nodes
        let validators: Vec<_> = nodes.values()
            .filter(|n| n.is_validator && n.status == NodeStatus::Healthy)
            .collect();

        // Get miner nodes
        let miners: Vec<_> = nodes.values()
            .filter(|n| !n.is_validator && n.status == NodeStatus::Healthy)
            .collect();

        for (table, policy) in policies.iter() {
            let mut table_assignments = Vec::new();

            if policy.shard_keys.is_empty() {
                // Full replication
                let required_copies = (nodes.len() as f32 * policy.replication_factor).ceil() as usize;
                let mut selected_nodes = HashSet::new();

                // First assign to validators
                for node in validators.iter() {
                    if selected_nodes.len() >= required_copies {
                        break;
                    }
                    selected_nodes.insert(node.addr);
                }

                // Then miners if allowed and needed
                if !policy.validator_only {
                    for node in miners.iter() {
                        if selected_nodes.len() >= required_copies {
                            break;
                        }
                        selected_nodes.insert(node.addr);
                    }
                }

                table_assignments.push(ShardAssignment {
                    table: table.clone(),
                    shard_id: 0,
                    range_start: None,
                    range_end: None,
                    primaries: selected_nodes.clone(),
                    secondaries: HashSet::new(),
                });
            } else {
                // Sharded replication
                let shard_count = (nodes.len() as f32 * policy.replication_factor).ceil() as u32;
                
                for shard_id in 0..shard_count {
                    let mut primaries = HashSet::new();
                    let mut secondaries = HashSet::new();

                    // Assign primary (validator only)
                    if let Some(primary) = validators.iter()
                        .filter(|n| n.load < self.config.load_threshold)
                        .min_by_key(|n| n.shards.len()) {
                        primaries.insert(primary.addr);
                    }

                    // Assign secondaries
                    let required_copies = policy.min_validator_copies as usize;
                    let available_nodes = if policy.validator_only {
                        &validators
                    } else {
                        &miners
                    };

                    for node in available_nodes.iter()
                        .filter(|n| n.load < self.config.load_threshold)
                        .take(required_copies - 1) {
                        secondaries.insert(node.addr);
                    }

                    table_assignments.push(ShardAssignment {
                        table: table.clone(),
                        shard_id,
                        range_start: Some(format!("{}", shard_id)),
                        range_end: Some(format!("{}", shard_id + 1)),
                        primaries,
                        secondaries,
                    });
                }
            }

            assignments.insert(table.clone(), table_assignments);
        }

        Ok(())
    }

    /// Start background tasks
    pub fn start(&self) {
        // Start rebalance task
        let config = self.config.clone();
        let shard_manager = self.clone();

        tokio::spawn(async move {
            let mut interval = tokio::time::interval(config.rebalance_interval);
            loop {
                interval.tick().await;
                if let Err(e) = shard_manager.rebalance_shards().await {
                    eprintln!("Error rebalancing shards: {}", e);
                }
            }
        });
    }
}

impl Clone for ShardManager {
    fn clone(&self) -> Self {
        Self {
            config: self.config.clone(),
            policies: self.policies.clone(),
            assignments: self.assignments.clone(),
            nodes: self.nodes.clone(),
            analyzer: SqlAnalyzer::new(super::DatabaseType::PostgreSQL),
            metrics: self.metrics.clone(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;

    #[tokio::test]
    async fn test_table_policy() {
        let metrics = Arc::new(MetricsCollector::new());
        let config = ShardConfig {
            default_replication_factor: 0.5,
            min_validator_copies: 2,
            max_shard_size: 1024 * 1024 * 1024,
            rebalance_interval: Duration::from_secs(3600),
            load_threshold: 0.8,
        };

        let manager = ShardManager::new(config, metrics);

        // Create policy for users table
        let policy = TablePolicy {
            table: "users".to_string(),
            replication_factor: 1.0, // Full replication
            validator_only: true,
            min_validator_copies: 2,
            read_roles: HashSet::from([NodeRole::Primary, NodeRole::Secondary]),
            write_roles: HashSet::from([NodeRole::Primary]),
            shard_keys: Vec::new(),
            range_sharding: false,
            shard_function: None,
        };

        manager.set_policy(policy.clone()).await.unwrap();
        let retrieved = manager.get_policy("users").await.unwrap();
        assert_eq!(retrieved.table, "users");
        assert_eq!(retrieved.replication_factor, 1.0);
    }

    #[tokio::test]
    async fn test_access_control() {
        let metrics = Arc::new(MetricsCollector::new());
        let config = ShardConfig::default();
        let manager = ShardManager::new(config, metrics);

        // Create policy
        let policy = TablePolicy {
            table: "users".to_string(),
            validator_only: true,
            read_roles: HashSet::from([NodeRole::Primary, NodeRole::Secondary]),
            write_roles: HashSet::from([NodeRole::Primary]),
            ..Default::default()
        };
        manager.set_policy(policy).await.unwrap();

        // Test validator primary
        let validator_primary = NodeShardInfo {
            addr: "127.0.0.1:8000".parse().unwrap(),
            role: NodeRole::Primary,
            status: NodeStatus::Healthy,
            disk_space: 1000000,
            load: 0.5,
            shards: HashSet::new(),
            is_validator: true,
        };
        assert!(manager.check_access(&validator_primary, "users", true).await);
        assert!(manager.check_access(&validator_primary, "users", false).await);

        // Test miner node
        let miner = NodeShardInfo {
            addr: "127.0.0.1:8001".parse().unwrap(),
            role: NodeRole::Secondary,
            status: NodeStatus::Healthy,
            disk_space: 1000000,
            load: 0.5,
            shards: HashSet::new(),
            is_validator: false,
        };
        assert!(!manager.check_access(&miner, "users", true).await);
        assert!(!manager.check_access(&miner, "users", false).await);
    }

    #[tokio::test]
    async fn test_shard_assignment() {
        let metrics = Arc::new(MetricsCollector::new());
        let config = ShardConfig::default();
        let manager = ShardManager::new(config, metrics);

        // Create sharded table policy
        let policy = TablePolicy {
            table: "events".to_string(),
            replication_factor: 0.5,
            validator_only: false,
            min_validator_copies: 1,
            shard_keys: vec!["user_id".to_string()],
            range_sharding: true,
            ..Default::default()
        };
        manager.set_policy(policy).await.unwrap();

        // Add some nodes
        for i in 0..4 {
            let node = NodeShardInfo {
                addr: format!("127.0.0.1:800{}", i).parse().unwrap(),
                role: if i < 2 { NodeRole::Primary } else { NodeRole::Secondary },
                status: NodeStatus::Healthy,
                disk_space: 1000000,
                load: 0.5,
                shards: HashSet::new(),
                is_validator: i < 2,
            };
            manager.update_node(node).await.unwrap();
        }

        // Rebalance shards
        manager.rebalance_shards().await.unwrap();

        // Check assignments
        let assignments = manager.assignments.read().await;
        let event_shards = assignments.get("events").unwrap();
        assert!(!event_shards.is_empty());
        
        // Each shard should have at least one primary
        for shard in event_shards {
            assert!(!shard.primaries.is_empty());
        }
    }
} 