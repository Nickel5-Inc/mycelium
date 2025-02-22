use std::net::SocketAddr;
use std::sync::Arc;
use tokio::sync::broadcast;
use tokio::time::{interval, Duration};
use serde::{Serialize, Deserialize};

use crate::database::{Error, Result};
use crate::network::{Network, PeerEvent};
use crate::protocol::Message as NetworkMessage;
use super::{VersionedEntry, NodeRole};
use super::heartbeat::{HeartbeatMonitor, HeartbeatConfig, HeartbeatEvent, NodeStatus};
use super::retry::{RetryManager, RetryConfig};
use super::metrics::MetricsCollector;

/// Transport message types
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum Message {
    /// WAL entry for replication
    WalEntry(VersionedEntry),
    /// Request WAL entries after sequence
    RequestEntries { after_sequence: u64 },
    /// Response with WAL entries
    ResponseEntries { entries: Vec<VersionedEntry> },
    /// Node status update
    Status { role: NodeRole, sequence: u64 },
    /// Heartbeat to keep connection alive
    Heartbeat { role: NodeRole, sequence: u64 },
}

/// Transport configuration
#[derive(Debug, Clone)]
pub struct TransportConfig {
    /// Maximum message size
    pub max_message_size: usize,
    /// Connection keep-alive interval
    pub keep_alive_interval: Duration,
    /// Heartbeat configuration
    pub heartbeat: HeartbeatConfig,
    /// Retry configuration
    pub retry: RetryConfig,
}

impl Default for TransportConfig {
    fn default() -> Self {
        Self {
            max_message_size: 16 * 1024 * 1024, // 16MB
            keep_alive_interval: Duration::from_secs(30),
            heartbeat: HeartbeatConfig::default(),
            retry: RetryConfig::default(),
        }
    }
}

/// Transport layer for database replication
#[derive(Debug)]
pub struct Transport {
    /// Network reference
    network: Arc<Network>,
    /// Message sender
    message_tx: broadcast::Sender<(SocketAddr, Message)>,
    /// Heartbeat monitor
    heartbeat: Arc<HeartbeatMonitor>,
    /// Retry manager
    retry: Arc<RetryManager>,
    /// Metrics collector
    metrics: Arc<MetricsCollector>,
    /// Local node role
    role: NodeRole,
    /// Current sequence number
    sequence: Arc<tokio::sync::RwLock<u64>>,
}

impl Transport {
    /// Create a new transport
    pub async fn new(network: Arc<Network>, config: TransportConfig, role: NodeRole) -> Result<Self> {
        let (message_tx, _) = broadcast::channel(1000);
        let message_tx_clone = message_tx.clone();
        let heartbeat = Arc::new(HeartbeatMonitor::new(config.heartbeat));
        let retry = Arc::new(RetryManager::new(network.clone(), config.retry));
        let metrics = Arc::new(MetricsCollector::new());
        let sequence = Arc::new(tokio::sync::RwLock::new(0));

        // Subscribe to network events
        let mut events = network.events();
        let heartbeat_clone = heartbeat.clone();
        let retry_clone = retry.clone();
        let metrics_clone = metrics.clone();
        let sequence_clone = sequence.clone();
        
        tokio::spawn(async move {
            while let Ok(event) = events.recv().await {
                match event {
                    PeerEvent::Message(msg) => {
                        // Handle only replication messages
                        if let NetworkMessage::Custom(data) = msg.payload {
                            if let Ok(repl_msg) = rmp_serde::from_slice::<Message>(&data) {
                                // Handle heartbeats
                                if let Message::Heartbeat { role, sequence } = repl_msg {
                                    if let Ok(lag) = heartbeat_clone.handle_heartbeat(msg.sender_addr, role, sequence).await {
                                        metrics_clone.update_node_status(
                                            msg.sender_addr,
                                            role,
                                            NodeStatus::Healthy,
                                            lag,
                                        ).await;
                                    }
                                }
                                // Update sequence for WAL entries
                                if let Message::WalEntry(entry) = &repl_msg {
                                    let mut seq = sequence_clone.write().await;
                                    *seq = std::cmp::max(*seq, entry.sequence());
                                    metrics_clone.record_wal_entry(msg.sender_addr, data.len() as u64).await;
                                }
                                let _ = message_tx_clone.send((msg.sender_addr, repl_msg));
                            }
                        }
                        // Handle successful connection
                        retry_clone.handle_connected(msg.sender_addr).await;
                    }
                    PeerEvent::Disconnected(addr) => {
                        // Start retrying connection
                        retry_clone.start_retrying(addr).await.ok();
                        metrics_clone.record_connection_failure().await;
                    }
                    _ => {}
                }
            }
        });

        // Start heartbeat sender
        let transport = Self {
            network: network.clone(),
            message_tx,
            heartbeat,
            retry,
            metrics,
            role,
            sequence,
        };
        transport.start_heartbeat(config.heartbeat.interval).await?;

        // Start health checker
        let heartbeat_clone = transport.heartbeat.clone();
        let retry_clone = transport.retry.clone();
        let metrics_clone = transport.metrics.clone();
        tokio::spawn(async move {
            let mut interval = interval(config.heartbeat.interval);
            loop {
                interval.tick().await;
                if let Ok(events) = heartbeat_clone.check_health().await {
                    for event in events {
                        if let HeartbeatEvent::StatusChange { addr, new_status, role, lag } = event {
                            metrics_clone.update_node_status(addr, role, new_status, lag).await;
                            if new_status == NodeStatus::Dead {
                                // Start retrying dead connections
                                retry_clone.start_retrying(addr).await.ok();
                            }
                        }
                    }
                }
            }
        });

        Ok(transport)
    }

    /// Start sending heartbeats
    async fn start_heartbeat(&self, interval: Duration) -> Result<()> {
        let network = self.network.clone();
        let role = self.role;
        let sequence = self.sequence.clone();

        tokio::spawn(async move {
            let mut interval = interval(interval);
            loop {
                interval.tick().await;
                let seq = *sequence.read().await;
                let msg = Message::Heartbeat { role, sequence: seq };
                
                // Serialize message
                if let Ok(data) = rmp_serde::to_vec(&msg) {
                    // Broadcast heartbeat
                    network.broadcast(NetworkMessage::Custom(data)).await.ok();
                }
            }
        });

        Ok(())
    }

    /// Send a message to a specific peer
    pub async fn send_to(&self, addr: SocketAddr, message: Message) -> Result<()> {
        // Update sequence for WAL entries
        if let Message::WalEntry(entry) = &message {
            let mut seq = self.sequence.write().await;
            *seq = std::cmp::max(*seq, entry.sequence());
        }

        let data = rmp_serde::to_vec(&message)
            .map_err(|e| Error::Replication(format!("Failed to serialize message: {}", e)))?;

        self.network.send_to(
            addr,
            NetworkMessage::Custom(data),
        ).await?;

        Ok(())
    }

    /// Broadcast a message to all peers
    pub async fn broadcast(&self, message: Message) -> Result<()> {
        // Update sequence for WAL entries
        if let Message::WalEntry(entry) = &message {
            let mut seq = self.sequence.write().await;
            *seq = std::cmp::max(*seq, entry.sequence());
        }

        let data = rmp_serde::to_vec(&message)
            .map_err(|e| Error::Replication(format!("Failed to serialize message: {}", e)))?;

        self.network.broadcast(
            NetworkMessage::Custom(data),
        ).await?;

        Ok(())
    }

    /// Subscribe to incoming messages
    pub fn subscribe(&self) -> broadcast::Receiver<(SocketAddr, Message)> {
        self.message_tx.subscribe()
    }

    /// Subscribe to heartbeat events
    pub fn subscribe_heartbeat(&self) -> broadcast::Receiver<HeartbeatEvent> {
        self.heartbeat.subscribe()
    }

    /// Subscribe to retry events
    pub fn subscribe_retry(&self) -> broadcast::Receiver<super::retry::RetryEvent> {
        self.retry.subscribe()
    }

    /// Get current primary node
    pub async fn get_primary(&self) -> Option<SocketAddr> {
        self.heartbeat.get_primary().await
    }

    /// Get node health information
    pub async fn get_node_health(&self, addr: SocketAddr) -> Option<super::heartbeat::NodeHealth> {
        self.heartbeat.get_node_health(addr).await
    }

    /// Get all node health information
    pub async fn get_all_nodes(&self) -> Vec<super::heartbeat::NodeHealth> {
        self.heartbeat.get_all_nodes().await
    }

    /// Get current sequence number
    pub async fn sequence(&self) -> u64 {
        *self.sequence.read().await
    }

    /// Stop retrying connections to a peer
    pub async fn stop_retrying(&self, addr: SocketAddr) {
        self.retry.stop_retrying(addr).await;
    }

    /// Get current metrics
    pub async fn get_metrics(&self) -> super::metrics::ReplicationMetrics {
        self.metrics.get_metrics().await
    }

    /// Get metrics for a specific node
    pub async fn get_node_metrics(&self, addr: SocketAddr) -> Option<super::metrics::NodeMetrics> {
        self.metrics.get_node_metrics(addr).await
    }

    /// Get metrics for all nodes
    pub async fn get_all_node_metrics(&self) -> Vec<super::metrics::NodeMetrics> {
        self.metrics.get_all_node_metrics().await
    }

    /// Get uptime in seconds
    pub fn uptime(&self) -> u64 {
        self.metrics.uptime()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;
    use tokio::time::sleep;
    use crate::database::replication::{VersionVector, ConflictStrategy};
    use crate::network::{NetworkConfig, TcpTransport};

    #[tokio::test]
    async fn test_transport_with_heartbeat() {
        // Create network configs
        let mut server_config = NetworkConfig::default();
        server_config.local_addr = "127.0.0.1:0".parse().unwrap();
        let server_network = Arc::new(Network::new(server_config.clone(), TcpTransport));

        let mut client_config = NetworkConfig::default();
        client_config.local_addr = "127.0.0.1:0".parse().unwrap();
        let client_network = Arc::new(Network::new(client_config, TcpTransport));

        // Start networks
        let server_network_clone = server_network.clone();
        let server_handle = tokio::spawn(async move {
            server_network_clone.start().await.unwrap();
        });

        let client_network_clone = client_network.clone();
        let client_handle = tokio::spawn(async move {
            client_network_clone.start().await.unwrap();
        });

        // Wait for networks to start
        sleep(Duration::from_millis(100)).await;

        // Create transports
        let transport_config = TransportConfig {
            max_message_size: 1024 * 1024,
            keep_alive_interval: Duration::from_secs(1),
            heartbeat: HeartbeatConfig {
                interval: Duration::from_millis(100),
                timeout: Duration::from_millis(300),
                failure_threshold: 2,
                lag_threshold: Duration::from_secs(5),
            },
            retry: RetryConfig::default(),
        };

        let server_transport = Transport::new(
            server_network.clone(),
            transport_config.clone(),
            NodeRole::Primary,
        ).await.unwrap();

        let client_transport = Transport::new(
            client_network.clone(),
            transport_config,
            NodeRole::Secondary,
        ).await.unwrap();

        // Connect client to server
        client_network.connect(server_config.local_addr).await.unwrap();

        // Subscribe to heartbeat events
        let mut server_heartbeat = server_transport.subscribe_heartbeat();

        // Wait for heartbeats
        sleep(Duration::from_millis(500)).await;

        // Verify heartbeat monitoring
        let server_nodes = server_transport.get_all_nodes().await;
        assert!(!server_nodes.is_empty(), "Should have registered nodes");

        let client_addr = client_network.local_addr().unwrap();
        if let Some(health) = server_transport.get_node_health(client_addr).await {
            assert_eq!(health.status, NodeStatus::Healthy);
            assert_eq!(health.role, NodeRole::Secondary);
        } else {
            panic!("Client node not found");
        }

        // Stop client and verify detection
        client_handle.abort();
        sleep(Duration::from_millis(500)).await;

        // Should see status changes
        let mut saw_status_change = false;
        while let Ok(event) = server_heartbeat.try_recv() {
            if let HeartbeatEvent::StatusChange { addr, new_status, .. } = event {
                if addr == client_addr && new_status == NodeStatus::Dead {
                    saw_status_change = true;
                    break;
                }
            }
        }
        assert!(saw_status_change, "Should have detected client failure");

        // Cleanup
        server_handle.abort();
    }
} 