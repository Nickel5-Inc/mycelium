use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;
use serde::{Serialize, Deserialize};

use super::{NodeRole, NodeStatus};

/// Replication metrics
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ReplicationMetrics {
    /// Total WAL entries processed
    pub wal_entries: u64,
    /// Total bytes replicated
    pub bytes_replicated: u64,
    /// Average replication lag in milliseconds
    pub avg_replication_lag: i64,
    /// Number of conflicts detected
    pub conflicts_detected: u64,
    /// Number of conflicts resolved
    pub conflicts_resolved: u64,
    /// Number of failed replication attempts
    pub replication_failures: u64,
    /// Number of successful replication attempts
    pub replication_successes: u64,
    /// Number of connection retries
    pub connection_retries: u64,
    /// Number of connection failures
    pub connection_failures: u64,
    /// Number of active connections
    pub active_connections: u64,
    /// Number of nodes by role
    pub nodes_by_role: HashMap<NodeRole, u64>,
    /// Number of nodes by status
    pub nodes_by_status: HashMap<NodeStatus, u64>,
}

impl Default for ReplicationMetrics {
    fn default() -> Self {
        Self {
            wal_entries: 0,
            bytes_replicated: 0,
            avg_replication_lag: 0,
            conflicts_detected: 0,
            conflicts_resolved: 0,
            replication_failures: 0,
            replication_successes: 0,
            connection_retries: 0,
            connection_failures: 0,
            active_connections: 0,
            nodes_by_role: HashMap::new(),
            nodes_by_status: HashMap::new(),
        }
    }
}

/// Node-specific metrics
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NodeMetrics {
    /// Node address
    pub addr: SocketAddr,
    /// Node role
    pub role: NodeRole,
    /// Node status
    pub status: NodeStatus,
    /// Current replication lag in milliseconds
    pub replication_lag: i64,
    /// Number of WAL entries processed
    pub wal_entries: u64,
    /// Number of bytes replicated
    pub bytes_replicated: u64,
    /// Number of connection retries
    pub connection_retries: u64,
    /// Last successful connection
    pub last_connection: Option<i64>,
    /// Last successful replication
    pub last_replication: Option<i64>,
}

/// Metrics collector for replication system
#[derive(Debug)]
pub struct MetricsCollector {
    /// Global metrics
    metrics: Arc<RwLock<ReplicationMetrics>>,
    /// Per-node metrics
    node_metrics: Arc<RwLock<HashMap<SocketAddr, NodeMetrics>>>,
    /// Metrics collection start time
    start_time: Instant,
}

impl MetricsCollector {
    /// Create a new metrics collector
    pub fn new() -> Self {
        Self {
            metrics: Arc::new(RwLock::new(ReplicationMetrics::default())),
            node_metrics: Arc::new(RwLock::new(HashMap::new())),
            start_time: Instant::now(),
        }
    }

    /// Record WAL entry replication
    pub async fn record_wal_entry(&self, addr: SocketAddr, bytes: u64) {
        let mut metrics = self.metrics.write().await;
        metrics.wal_entries += 1;
        metrics.bytes_replicated += bytes;
        metrics.replication_successes += 1;

        let mut node_metrics = self.node_metrics.write().await;
        if let Some(node) = node_metrics.get_mut(&addr) {
            node.wal_entries += 1;
            node.bytes_replicated += bytes;
            node.last_replication = Some(chrono::Utc::now().timestamp_millis());
        }
    }

    /// Record replication failure
    pub async fn record_failure(&self, addr: SocketAddr) {
        let mut metrics = self.metrics.write().await;
        metrics.replication_failures += 1;
    }

    /// Record conflict detection
    pub async fn record_conflict(&self, resolved: bool) {
        let mut metrics = self.metrics.write().await;
        metrics.conflicts_detected += 1;
        if resolved {
            metrics.conflicts_resolved += 1;
        }
    }

    /// Record connection retry
    pub async fn record_retry(&self, addr: SocketAddr) {
        let mut metrics = self.metrics.write().await;
        metrics.connection_retries += 1;

        let mut node_metrics = self.node_metrics.write().await;
        if let Some(node) = node_metrics.get_mut(&addr) {
            node.connection_retries += 1;
        }
    }

    /// Record connection failure
    pub async fn record_connection_failure(&self) {
        let mut metrics = self.metrics.write().await;
        metrics.connection_failures += 1;
    }

    /// Update node status
    pub async fn update_node_status(&self, addr: SocketAddr, role: NodeRole, status: NodeStatus, lag: Duration) {
        let mut metrics = self.metrics.write().await;
        
        // Update role counts
        *metrics.nodes_by_role.entry(role).or_default() += 1;
        *metrics.nodes_by_status.entry(status).or_default() += 1;

        // Update node metrics
        let mut node_metrics = self.node_metrics.write().await;
        let node = node_metrics.entry(addr).or_insert_with(|| NodeMetrics {
            addr,
            role,
            status,
            replication_lag: 0,
            wal_entries: 0,
            bytes_replicated: 0,
            connection_retries: 0,
            last_connection: None,
            last_replication: None,
        });

        node.role = role;
        node.status = status;
        node.replication_lag = lag.as_millis() as i64;

        if status == NodeStatus::Healthy {
            node.last_connection = Some(chrono::Utc::now().timestamp_millis());
        }

        // Update average replication lag
        let active_nodes: Vec<_> = node_metrics.values()
            .filter(|n| n.status == NodeStatus::Healthy)
            .collect();
        
        if !active_nodes.is_empty() {
            metrics.avg_replication_lag = active_nodes.iter()
                .map(|n| n.replication_lag)
                .sum::<i64>() / active_nodes.len() as i64;
            metrics.active_connections = active_nodes.len() as u64;
        }
    }

    /// Get current metrics
    pub async fn get_metrics(&self) -> ReplicationMetrics {
        self.metrics.read().await.clone()
    }

    /// Get metrics for a specific node
    pub async fn get_node_metrics(&self, addr: SocketAddr) -> Option<NodeMetrics> {
        self.node_metrics.read().await.get(&addr).cloned()
    }

    /// Get metrics for all nodes
    pub async fn get_all_node_metrics(&self) -> Vec<NodeMetrics> {
        self.node_metrics.read().await.values().cloned().collect()
    }

    /// Get uptime in seconds
    pub fn uptime(&self) -> u64 {
        self.start_time.elapsed().as_secs()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::time::sleep;

    #[tokio::test]
    async fn test_metrics_collector() {
        let collector = MetricsCollector::new();
        let addr = "127.0.0.1:8080".parse().unwrap();

        // Test WAL entry recording
        collector.record_wal_entry(addr, 1000).await;
        let metrics = collector.get_metrics().await;
        assert_eq!(metrics.wal_entries, 1);
        assert_eq!(metrics.bytes_replicated, 1000);
        assert_eq!(metrics.replication_successes, 1);

        // Test conflict recording
        collector.record_conflict(true).await;
        let metrics = collector.get_metrics().await;
        assert_eq!(metrics.conflicts_detected, 1);
        assert_eq!(metrics.conflicts_resolved, 1);

        // Test node status updates
        collector.update_node_status(
            addr,
            NodeRole::Primary,
            NodeStatus::Healthy,
            Duration::from_millis(100),
        ).await;

        let node_metrics = collector.get_node_metrics(addr).await.unwrap();
        assert_eq!(node_metrics.role, NodeRole::Primary);
        assert_eq!(node_metrics.status, NodeStatus::Healthy);
        assert_eq!(node_metrics.replication_lag, 100);

        // Test retry recording
        collector.record_retry(addr).await;
        let metrics = collector.get_metrics().await;
        assert_eq!(metrics.connection_retries, 1);

        let node_metrics = collector.get_node_metrics(addr).await.unwrap();
        assert_eq!(node_metrics.connection_retries, 1);

        // Test uptime
        sleep(Duration::from_secs(1)).await;
        assert!(collector.uptime() >= 1);
    }
} 