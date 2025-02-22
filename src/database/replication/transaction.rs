use std::collections::{HashMap, HashSet};
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::{broadcast, RwLock};
use tokio::time::interval;
use serde::{Serialize, Deserialize};
use uuid::Uuid;
use chrono::{DateTime, Utc};

use crate::database::{Error, Result, DatabaseType};
use super::sql::{SqlAnalyzer, TableOperation, OperationType};
use super::metrics::MetricsCollector;
use super::transport::{Transport, Message};
use super::shard::{ShardManager, ShardAssignment};
use super::{NodeRole, NodeStatus};

/// Transaction state
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum TransactionState {
    /// Transaction is active
    Active,
    /// Transaction is preparing to commit (phase 1)
    Preparing,
    /// Transaction is committing (phase 2)
    Committing,
    /// Transaction is committed
    Committed,
    /// Transaction is rolling back
    RollingBack,
    /// Transaction is rolled back
    RolledBack,
    /// Transaction is aborted
    Aborted,
}

/// Transaction operation
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TransactionOp {
    /// SQL statement
    pub sql: String,
    /// Query parameters
    pub params: Vec<Vec<u8>>,
    /// Target shard
    pub shard: Option<ShardAssignment>,
    /// Operation timestamp
    pub timestamp: i64,
}

/// Transaction participant
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Participant {
    /// Node address
    pub addr: SocketAddr,
    /// Node role
    pub role: NodeRole,
    /// Shard assignment
    pub shard: ShardAssignment,
    /// Vote status
    pub vote: Option<bool>,
    /// Prepare timestamp
    pub prepare_time: Option<i64>,
    /// Commit timestamp
    pub commit_time: Option<i64>,
}

/// Distributed transaction
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Transaction {
    /// Transaction ID
    pub id: Uuid,
    /// Coordinator address
    pub coordinator: SocketAddr,
    /// Transaction state
    pub state: TransactionState,
    /// Start timestamp
    pub start_time: i64,
    /// Operations in this transaction
    pub operations: Vec<TransactionOp>,
    /// Participating nodes
    pub participants: HashMap<SocketAddr, Participant>,
    /// Transaction timeout
    pub timeout: Duration,
}

/// Transaction message types
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum TransactionMessage {
    /// Begin transaction
    Begin {
        /// Transaction ID
        tx_id: Uuid,
        /// Coordinator address
        coordinator: SocketAddr,
        /// Transaction timeout
        timeout: Duration,
    },
    /// Prepare to commit (phase 1)
    Prepare {
        /// Transaction ID
        tx_id: Uuid,
        /// Operations to prepare
        operations: Vec<TransactionOp>,
    },
    /// Vote for transaction (phase 1 response)
    Vote {
        /// Transaction ID
        tx_id: Uuid,
        /// Participant address
        participant: SocketAddr,
        /// Vote (true = commit, false = abort)
        vote: bool,
    },
    /// Commit transaction (phase 2)
    Commit {
        /// Transaction ID
        tx_id: Uuid,
    },
    /// Abort transaction
    Abort {
        /// Transaction ID
        tx_id: Uuid,
        /// Reason for abort
        reason: String,
    },
    /// Acknowledge commit/abort
    Ack {
        /// Transaction ID
        tx_id: Uuid,
        /// Participant address
        participant: SocketAddr,
        /// Success status
        success: bool,
    },
}

/// Transaction manager configuration
#[derive(Debug, Clone)]
pub struct TransactionConfig {
    /// Default transaction timeout
    pub default_timeout: Duration,
    /// Prepare timeout
    pub prepare_timeout: Duration,
    /// Commit timeout
    pub commit_timeout: Duration,
    /// Transaction cleanup interval
    pub cleanup_interval: Duration,
    /// Maximum concurrent transactions
    pub max_transactions: usize,
}

impl Default for TransactionConfig {
    fn default() -> Self {
        Self {
            default_timeout: Duration::from_secs(30),
            prepare_timeout: Duration::from_secs(5),
            commit_timeout: Duration::from_secs(5),
            cleanup_interval: Duration::from_secs(60),
            max_transactions: 1000,
        }
    }
}

/// Transaction manager handles distributed transactions
pub struct TransactionManager {
    /// Configuration
    config: TransactionConfig,
    /// Transport layer
    transport: Arc<Transport>,
    /// Shard manager
    shard_manager: Arc<ShardManager>,
    /// Active transactions
    transactions: Arc<RwLock<HashMap<Uuid, Transaction>>>,
    /// SQL analyzer
    analyzer: SqlAnalyzer,
    /// Metrics collector
    metrics: Arc<MetricsCollector>,
    /// Event sender
    event_tx: broadcast::Sender<TransactionEvent>,
}

/// Transaction events
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum TransactionEvent {
    /// Transaction state changed
    StateChanged {
        /// Transaction ID
        tx_id: Uuid,
        /// Old state
        old_state: TransactionState,
        /// New state
        new_state: TransactionState,
    },
    /// Transaction completed
    Completed {
        /// Transaction ID
        tx_id: Uuid,
        /// Success status
        success: bool,
        /// Duration in milliseconds
        duration: i64,
    },
    /// Transaction error
    Error {
        /// Transaction ID
        tx_id: Uuid,
        /// Error message
        error: String,
    },
}

impl TransactionManager {
    /// Create a new transaction manager
    pub fn new(
        config: TransactionConfig,
        transport: Arc<Transport>,
        shard_manager: Arc<ShardManager>,
        metrics: Arc<MetricsCollector>,
    ) -> Self {
        let (event_tx, _) = broadcast::channel(100);
        Self {
            config,
            transport,
            shard_manager,
            transactions: Arc::new(RwLock::new(HashMap::new())),
            analyzer: SqlAnalyzer::new(super::DatabaseType::PostgreSQL),
            metrics,
            event_tx,
        }
    }

    /// Begin a new distributed transaction
    pub async fn begin(&self) -> Result<Uuid> {
        let tx_id = Uuid::new_v4();
        let coordinator = self.transport.local_addr();
        let now = chrono::Utc::now().timestamp_millis();

        let transaction = Transaction {
            id: tx_id,
            coordinator,
            state: TransactionState::Active,
            start_time: now,
            operations: Vec::new(),
            participants: HashMap::new(),
            timeout: self.config.default_timeout,
        };

        // Store transaction
        let mut transactions = self.transactions.write().await;
        if transactions.len() >= self.config.max_transactions {
            return Err(Error::Transaction("too many active transactions".into()));
        }
        transactions.insert(tx_id, transaction);

        // Broadcast begin message
        let msg = TransactionMessage::Begin {
            tx_id,
            coordinator,
            timeout: self.config.default_timeout,
        };
        self.broadcast_transaction(msg).await?;

        Ok(tx_id)
    }

    /// Add operation to transaction
    pub async fn add_operation(&self, tx_id: Uuid, sql: String, params: Vec<Vec<u8>>) -> Result<()> {
        let mut transactions = self.transactions.write().await;
        let transaction = transactions.get_mut(&tx_id)
            .ok_or_else(|| Error::Transaction("transaction not found".into()))?;

        if transaction.state != TransactionState::Active {
            return Err(Error::Transaction("transaction is not active".into()));
        }

        // Analyze query to determine shards
        let shard = if let Some(table) = self.analyzer.extract_table(&sql)? {
            self.shard_manager.get_shard(&table, &sql).await?
        } else {
            None
        };

        // Add operation
        let op = TransactionOp {
            sql,
            params,
            shard,
            timestamp: chrono::Utc::now().timestamp_millis(),
        };
        transaction.operations.push(op);

        Ok(())
    }

    /// Commit transaction (start two-phase commit)
    pub async fn commit(&self, tx_id: Uuid) -> Result<()> {
        let mut transactions = self.transactions.write().await;
        let transaction = transactions.get_mut(&tx_id)
            .ok_or_else(|| Error::Transaction("transaction not found".into()))?;

        if transaction.state != TransactionState::Active {
            return Err(Error::Transaction("transaction is not active".into()));
        }

        // Update state
        let old_state = transaction.state;
        transaction.state = TransactionState::Preparing;
        self.emit_state_change(tx_id, old_state, TransactionState::Preparing).await;

        // Send prepare messages
        let msg = TransactionMessage::Prepare {
            tx_id,
            operations: transaction.operations.clone(),
        };
        self.broadcast_transaction(msg).await?;

        // Start prepare timeout
        let tx_manager = self.clone();
        tokio::spawn(async move {
            tokio::time::sleep(tx_manager.config.prepare_timeout).await;
            tx_manager.check_prepare_timeout(tx_id).await;
        });

        Ok(())
    }

    /// Handle prepare timeout
    async fn check_prepare_timeout(&self, tx_id: Uuid) {
        let mut transactions = self.transactions.write().await;
        if let Some(transaction) = transactions.get_mut(&tx_id) {
            if transaction.state == TransactionState::Preparing {
                // Not all participants voted in time
                transaction.state = TransactionState::Aborted;
                self.emit_state_change(tx_id, TransactionState::Preparing, TransactionState::Aborted).await;
                
                // Send abort message
                let msg = TransactionMessage::Abort {
                    tx_id,
                    reason: "prepare timeout".into(),
                };
                self.broadcast_transaction(msg).await.ok();
            }
        }
    }

    /// Handle vote from participant
    async fn handle_vote(&self, tx_id: Uuid, participant: SocketAddr, vote: bool) -> Result<()> {
        let mut transactions = self.transactions.write().await;
        let transaction = transactions.get_mut(&tx_id)
            .ok_or_else(|| Error::Transaction("transaction not found".into()))?;

        if transaction.state != TransactionState::Preparing {
            return Ok(());
        }

        // Record vote
        if let Some(p) = transaction.participants.get_mut(&participant) {
            p.vote = Some(vote);
            p.prepare_time = Some(chrono::Utc::now().timestamp_millis());
        }

        // Check if all participants have voted
        let all_voted = transaction.participants.values()
            .all(|p| p.vote.is_some());

        if all_voted {
            // Check if all votes are positive
            let all_positive = transaction.participants.values()
                .all(|p| p.vote == Some(true));

            if all_positive {
                // All participants voted to commit
                transaction.state = TransactionState::Committing;
                self.emit_state_change(tx_id, TransactionState::Preparing, TransactionState::Committing).await;

                // Send commit message
                let msg = TransactionMessage::Commit { tx_id };
                self.broadcast_transaction(msg).await?;

                // Start commit timeout
                let tx_manager = self.clone();
                tokio::spawn(async move {
                    tokio::time::sleep(tx_manager.config.commit_timeout).await;
                    tx_manager.check_commit_timeout(tx_id).await;
                });
            } else {
                // At least one participant voted to abort
                transaction.state = TransactionState::Aborted;
                self.emit_state_change(tx_id, TransactionState::Preparing, TransactionState::Aborted).await;

                // Send abort message
                let msg = TransactionMessage::Abort {
                    tx_id,
                    reason: "participant voted to abort".into(),
                };
                self.broadcast_transaction(msg).await?;
            }
        }

        Ok(())
    }

    /// Handle commit acknowledgment
    async fn handle_ack(&self, tx_id: Uuid, participant: SocketAddr, success: bool) -> Result<()> {
        let mut transactions = self.transactions.write().await;
        let transaction = transactions.get_mut(&tx_id)
            .ok_or_else(|| Error::Transaction("transaction not found".into()))?;

        if transaction.state != TransactionState::Committing {
            return Ok(());
        }

        // Record acknowledgment
        if let Some(p) = transaction.participants.get_mut(&participant) {
            p.commit_time = Some(chrono::Utc::now().timestamp_millis());
        }

        // Check if all participants have acknowledged
        let all_acked = transaction.participants.values()
            .all(|p| p.commit_time.is_some());

        if all_acked {
            // Transaction is complete
            transaction.state = TransactionState::Committed;
            self.emit_state_change(tx_id, TransactionState::Committing, TransactionState::Committed).await;

            // Emit completion event
            let duration = chrono::Utc::now().timestamp_millis() - transaction.start_time;
            self.event_tx.send(TransactionEvent::Completed {
                tx_id,
                success: true,
                duration,
            }).ok();
        }

        Ok(())
    }

    /// Broadcast transaction message
    async fn broadcast_transaction(&self, msg: TransactionMessage) -> Result<()> {
        let data = rmp_serde::to_vec(&msg)
            .map_err(|e| Error::Transaction(format!("failed to serialize message: {}", e)))?;
        self.transport.broadcast(Message::Custom(data)).await?;
        Ok(())
    }

    /// Emit state change event
    async fn emit_state_change(&self, tx_id: Uuid, old_state: TransactionState, new_state: TransactionState) {
        self.event_tx.send(TransactionEvent::StateChanged {
            tx_id,
            old_state,
            new_state,
        }).ok();
    }

    /// Start background tasks
    pub fn start(&self) {
        // Start cleanup task
        let config = self.config.clone();
        let transactions = self.transactions.clone();

        tokio::spawn(async move {
            let mut interval = interval(config.cleanup_interval);
            loop {
                interval.tick().await;
                let mut txns = transactions.write().await;
                let now = chrono::Utc::now().timestamp_millis();

                // Remove completed transactions
                txns.retain(|_, tx| {
                    match tx.state {
                        TransactionState::Committed | TransactionState::RolledBack | TransactionState::Aborted => {
                            // Keep recent transactions for a while for query purposes
                            now - tx.start_time < config.cleanup_interval.as_millis() as i64
                        }
                        _ => true
                    }
                });
            }
        });
    }

    /// Subscribe to transaction events
    pub fn subscribe(&self) -> broadcast::Receiver<TransactionEvent> {
        self.event_tx.subscribe()
    }
}

impl Clone for TransactionManager {
    fn clone(&self) -> Self {
        Self {
            config: self.config.clone(),
            transport: self.transport.clone(),
            shard_manager: self.shard_manager.clone(),
            transactions: self.transactions.clone(),
            analyzer: SqlAnalyzer::new(super::DatabaseType::PostgreSQL),
            metrics: self.metrics.clone(),
            event_tx: self.event_tx.clone(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;
    use tokio::time::sleep;
    use crate::network::{Network, NetworkConfig, TcpTransport};

    #[tokio::test]
    async fn test_transaction_lifecycle() {
        // Create network and transport
        let mut config = NetworkConfig::default();
        config.local_addr = "127.0.0.1:0".parse().unwrap();
        let network = Arc::new(Network::new(config.clone(), TcpTransport));
        let transport = Arc::new(Transport::new(network.clone(), Default::default(), NodeRole::Primary).await.unwrap());

        // Create managers
        let metrics = Arc::new(MetricsCollector::new());
        let shard_manager = Arc::new(ShardManager::new(Default::default(), metrics.clone()));
        
        let tx_config = TransactionConfig {
            default_timeout: Duration::from_secs(5),
            prepare_timeout: Duration::from_secs(1),
            commit_timeout: Duration::from_secs(1),
            cleanup_interval: Duration::from_secs(10),
            max_transactions: 100,
        };
        let tx_manager = TransactionManager::new(tx_config, transport, shard_manager, metrics);

        // Start transaction
        let tx_id = tx_manager.begin().await.unwrap();

        // Add operations
        tx_manager.add_operation(
            tx_id,
            "UPDATE users SET name = 'test' WHERE id = 1".into(),
            vec![],
        ).await.unwrap();

        // Commit
        tx_manager.commit(tx_id).await.unwrap();

        // Wait for completion
        sleep(Duration::from_millis(100)).await;

        // Verify final state
        let transactions = tx_manager.transactions.read().await;
        let transaction = transactions.get(&tx_id).unwrap();
        assert!(matches!(
            transaction.state,
            TransactionState::Committed | TransactionState::Aborted
        ));
    }
} 