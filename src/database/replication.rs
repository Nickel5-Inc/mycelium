use std::sync::Arc;
use tokio::sync::{broadcast, RwLock};
use chrono::{DateTime, Utc};
use serde::{Serialize, Deserialize};
use uuid::Uuid;

use super::{Database, Error, Result, Value};

/// Replication node role
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum NodeRole {
    /// Primary node that accepts writes
    Primary,
    /// Secondary node that only accepts reads
    Secondary,
}

/// Write-ahead log entry
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WalEntry {
    /// Unique entry ID
    pub id: Uuid,
    /// Entry timestamp
    pub timestamp: DateTime<Utc>,
    /// SQL statement
    pub sql: String,
    /// Query parameters
    pub params: Vec<Value>,
    /// Transaction ID if part of a transaction
    pub transaction_id: Option<Uuid>,
    /// Entry sequence number
    pub sequence: u64,
    /// Node that created this entry
    pub source_node: String,
}

/// Replication state
#[derive(Debug)]
pub struct ReplicationState {
    /// Node role
    role: NodeRole,
    /// Node identifier
    node_id: String,
    /// Current sequence number
    sequence: u64,
    /// WAL entries channel
    wal_tx: broadcast::Sender<WalEntry>,
    /// Database reference
    db: Arc<dyn Database>,
}

impl ReplicationState {
    /// Create a new replication state
    pub fn new(role: NodeRole, node_id: String, db: Arc<dyn Database>) -> Self {
        let (wal_tx, _) = broadcast::channel(1000);
        Self {
            role,
            node_id,
            sequence: 0,
            wal_tx,
            db,
        }
    }

    /// Get current node role
    pub fn role(&self) -> NodeRole {
        self.role
    }

    /// Get node ID
    pub fn node_id(&self) -> &str {
        &self.node_id
    }

    /// Get current sequence number
    pub fn sequence(&self) -> u64 {
        self.sequence
    }

    /// Subscribe to WAL entries
    pub fn subscribe(&self) -> broadcast::Receiver<WalEntry> {
        self.wal_tx.subscribe()
    }

    /// Apply a WAL entry
    pub async fn apply_wal(&mut self, entry: WalEntry) -> Result<()> {
        if entry.sequence != self.sequence + 1 {
            return Err(Error::Replication(format!(
                "Expected sequence {}, got {}",
                self.sequence + 1,
                entry.sequence
            )));
        }

        // Apply the entry within a transaction
        self.db
            .with_tx(|tx| {
                Box::pin(async move {
                    // Execute the SQL
                    let params: Vec<&(dyn super::ToSql + Sync)> = entry
                        .params
                        .iter()
                        .map(|v| v as &(dyn super::ToSql + Sync))
                        .collect();
                    tx.execute(&entry.sql, &params).await?;

                    // Record the WAL entry
                    tx.execute(
                        "INSERT INTO _replication_log (
                            id, timestamp, sql, params, transaction_id, sequence, source_node
                        ) VALUES ($1, $2, $3, $4, $5, $6, $7)",
                        &[
                            &entry.id.to_string(),
                            &entry.timestamp,
                            &entry.sql,
                            &serde_json::to_string(&entry.params).unwrap(),
                            &entry.transaction_id.map(|id| id.to_string()),
                            &(entry.sequence as i64),
                            &entry.source_node,
                        ],
                    )
                    .await?;

                    Ok(())
                })
            })
            .await?;

        // Update sequence and broadcast
        self.sequence = entry.sequence;
        let _ = self.wal_tx.send(entry);

        Ok(())
    }

    /// Initialize replication tracking table
    pub async fn initialize(&self) -> Result<()> {
        self.db
            .execute(
                "CREATE TABLE IF NOT EXISTS _replication_log (
                    id TEXT PRIMARY KEY,
                    timestamp TIMESTAMP NOT NULL,
                    sql TEXT NOT NULL,
                    params TEXT NOT NULL,
                    transaction_id TEXT,
                    sequence BIGINT NOT NULL,
                    source_node TEXT NOT NULL,
                    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
                )",
                &[],
            )
            .await?;

        // Create index on sequence
        self.db
            .execute(
                "CREATE INDEX IF NOT EXISTS idx_replication_log_sequence 
                 ON _replication_log (sequence)",
                &[],
            )
            .await?;

        Ok(())
    }

    /// Get WAL entries after a sequence number
    pub async fn get_wal_entries(&self, after_sequence: u64) -> Result<Vec<WalEntry>> {
        let rows = self
            .db
            .query(
                "SELECT id, timestamp, sql, params, transaction_id, sequence, source_node
                 FROM _replication_log 
                 WHERE sequence > $1
                 ORDER BY sequence",
                &[&(after_sequence as i64)],
            )
            .await?;

        let mut entries = Vec::with_capacity(rows.len());
        for row in rows {
            entries.push(WalEntry {
                id: Uuid::parse_str(&row.get::<String>("id")?).unwrap(),
                timestamp: row.get("timestamp")?,
                sql: row.get("sql")?,
                params: serde_json::from_str(&row.get::<String>("params")?).unwrap(),
                transaction_id: row
                    .get::<Option<String>>("transaction_id")?
                    .map(|s| Uuid::parse_str(&s).unwrap()),
                sequence: row.get::<i64>("sequence")? as u64,
                source_node: row.get("source_node")?,
            });
        }

        Ok(entries)
    }
}

/// Replication manager handles coordinating replication between nodes
#[derive(Debug)]
pub struct ReplicationManager {
    /// Replication state
    state: Arc<RwLock<ReplicationState>>,
}

impl ReplicationManager {
    /// Create a new replication manager
    pub fn new(state: ReplicationState) -> Self {
        Self {
            state: Arc::new(RwLock::new(state)),
        }
    }

    /// Start replication manager
    pub async fn start(&self) -> Result<()> {
        // Initialize tables
        self.state.read().await.initialize().await?;

        // Start background tasks
        let state = self.state.clone();
        tokio::spawn(async move {
            Self::catchup_task(state).await;
        });

        Ok(())
    }

    /// Background task to catch up with primary
    async fn catchup_task(state: Arc<RwLock<ReplicationState>>) {
        let mut interval = tokio::time::interval(std::time::Duration::from_secs(1));

        loop {
            interval.tick().await;

            let mut state = state.write().await;
            if state.role() == NodeRole::Secondary {
                // Get entries we're missing
                match state.get_wal_entries(state.sequence()).await {
                    Ok(entries) => {
                        for entry in entries {
                            if let Err(e) = state.apply_wal(entry).await {
                                eprintln!("Error applying WAL entry: {}", e);
                                break;
                            }
                        }
                    }
                    Err(e) => {
                        eprintln!("Error getting WAL entries: {}", e);
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::database::{create_database, DatabaseConfig, DatabaseType};
    use std::time::Duration;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_replication() {
        // Create primary database
        let dir = tempdir().unwrap();
        let primary_path = dir.path().join("primary.db");
        let primary_config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(primary_path.to_str().unwrap().to_string()),
            idle_timeout: Duration::from_secs(5),
            connect_timeout: Duration::from_secs(5),
            max_connections: 1,
            ..Default::default()
        };
        let primary_db = Arc::new(create_database(primary_config).await.unwrap());

        // Create secondary database
        let secondary_path = dir.path().join("secondary.db");
        let secondary_config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(secondary_path.to_str().unwrap().to_string()),
            idle_timeout: Duration::from_secs(5),
            connect_timeout: Duration::from_secs(5),
            max_connections: 1,
            ..Default::default()
        };
        let secondary_db = Arc::new(create_database(secondary_config).await.unwrap());

        // Create replication managers
        let primary_state = ReplicationState::new(
            NodeRole::Primary,
            "primary".to_string(),
            primary_db.clone(),
        );
        let primary = ReplicationManager::new(primary_state);
        primary.start().await.unwrap();

        let secondary_state = ReplicationState::new(
            NodeRole::Secondary,
            "secondary".to_string(),
            secondary_db.clone(),
        );
        let secondary = ReplicationManager::new(secondary_state);
        secondary.start().await.unwrap();

        // Create test table on primary
        primary_db
            .execute(
                "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)",
                &[],
            )
            .await
            .unwrap();

        // Insert data through WAL
        let mut primary_state = primary.state.write().await;
        let entry = WalEntry {
            id: Uuid::new_v4(),
            timestamp: Utc::now(),
            sql: "INSERT INTO test (id, value) VALUES (1, 'test')".to_string(),
            params: vec![],
            transaction_id: None,
            sequence: primary_state.sequence() + 1,
            source_node: "primary".to_string(),
        };
        primary_state.apply_wal(entry).await.unwrap();

        // Wait for replication
        tokio::time::sleep(Duration::from_secs(2)).await;

        // Verify data on secondary
        let rows = secondary_db
            .query("SELECT value FROM test WHERE id = 1", &[])
            .await
            .unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].get::<String>("value").unwrap(), "test");
    }
} 