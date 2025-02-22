use std::collections::HashMap;
use serde::{Serialize, Deserialize};
use uuid::Uuid;
use chrono::{DateTime, Utc};

use crate::database::{Error, Result, DatabaseType};
use super::sql::{SqlAnalyzer, TableOperation};

/// Version vector for tracking causality
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct VersionVector {
    /// Node version counters
    counters: HashMap<String, u64>,
}

impl VersionVector {
    /// Create a new version vector
    pub fn new() -> Self {
        Self {
            counters: HashMap::new(),
        }
    }

    /// Increment version for a node
    pub fn increment(&mut self, node_id: &str) {
        *self.counters.entry(node_id.to_string()).or_insert(0) += 1;
    }

    /// Check if this version vector is concurrent with another
    pub fn is_concurrent_with(&self, other: &VersionVector) -> bool {
        !(self.happens_before(other) || other.happens_before(self))
    }

    /// Check if this version vector happens before another
    pub fn happens_before(&self, other: &VersionVector) -> bool {
        // Must be less than or equal in all dimensions
        let mut strictly_less = false;
        
        for (node, &counter) in &self.counters {
            match other.counters.get(node) {
                Some(&other_counter) => {
                    if counter > other_counter {
                        return false;
                    }
                    if counter < other_counter {
                        strictly_less = true;
                    }
                }
                None => {
                    if counter > 0 {
                        return false;
                    }
                }
            }
        }

        // And strictly less in at least one dimension
        strictly_less || other.counters.iter().any(|(node, &counter)| {
            counter > 0 && !self.counters.contains_key(node)
        })
    }

    /// Merge with another version vector
    pub fn merge(&mut self, other: &VersionVector) {
        for (node, &counter) in &other.counters {
            let entry = self.counters.entry(node.clone()).or_insert(0);
            *entry = std::cmp::max(*entry, counter);
        }
    }
}

/// Conflict resolution strategy
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ConflictStrategy {
    /// Last write wins based on timestamp
    LastWriteWins,
    /// First write wins based on timestamp
    FirstWriteWins,
    /// Higher sequence number wins
    HigherSequenceWins,
    /// Custom merge function
    Custom,
}

/// Versioned WAL entry with conflict detection
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VersionedEntry {
    /// Entry ID
    pub id: Uuid,
    /// Entry timestamp
    pub timestamp: DateTime<Utc>,
    /// SQL statement
    pub sql: String,
    /// Version vector
    pub version: VersionVector,
    /// Conflict resolution strategy
    pub strategy: ConflictStrategy,
    /// Previous entry IDs this entry depends on
    pub dependencies: Vec<Uuid>,
    /// Database type for SQL parsing
    #[serde(skip)]
    db_type: Option<DatabaseType>,
}

impl VersionedEntry {
    /// Create a new versioned entry
    pub fn new(
        sql: String,
        version: VersionVector,
        strategy: ConflictStrategy,
        dependencies: Vec<Uuid>,
    ) -> Self {
        Self {
            id: Uuid::new_v4(),
            timestamp: Utc::now(),
            sql,
            version,
            strategy,
            dependencies,
            db_type: None,
        }
    }

    /// Set database type for SQL parsing
    pub fn with_db_type(mut self, db_type: DatabaseType) -> Self {
        self.db_type = Some(db_type);
        self
    }

    /// Check if this entry conflicts with another
    pub fn conflicts_with(&self, other: &VersionedEntry) -> Result<bool> {
        // If entries are causally related, no conflict
        if self.version.happens_before(&other.version) || 
           other.version.happens_before(&self.version) {
            return Ok(false);
        }

        // If entries have dependencies on each other, no conflict
        if self.dependencies.contains(&other.id) ||
           other.dependencies.contains(&self.id) {
            return Ok(false);
        }

        // If we have database type, use SQL analysis
        if let Some(db_type) = self.db_type {
            let analyzer = SqlAnalyzer::new(db_type);
            analyzer.check_conflicts(&self.sql, &other.sql)
        } else {
            // Fall back to assuming concurrent operations conflict
            Ok(true)
        }
    }

    /// Resolve conflict with another entry
    pub fn resolve_conflict(&self, other: &VersionedEntry) -> Result<VersionedEntry> {
        // First check if there's actually a conflict
        if !self.conflicts_with(other)? {
            return Ok(self.clone());
        }

        match self.strategy {
            ConflictStrategy::LastWriteWins => {
                if self.timestamp > other.timestamp {
                    Ok(self.clone())
                } else {
                    Ok(other.clone())
                }
            }
            ConflictStrategy::FirstWriteWins => {
                if self.timestamp < other.timestamp {
                    Ok(self.clone())
                } else {
                    Ok(other.clone())
                }
            }
            ConflictStrategy::HigherSequenceWins => {
                let self_seq = self.version.counters.values().sum::<u64>();
                let other_seq = other.version.counters.values().sum::<u64>();
                if self_seq >= other_seq {
                    Ok(self.clone())
                } else {
                    Ok(other.clone())
                }
            }
            ConflictStrategy::Custom => {
                Err(Error::Replication("Custom conflict resolution not implemented".into()))
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_version_vector() {
        let mut v1 = VersionVector::new();
        let mut v2 = VersionVector::new();

        // Initially equal
        assert!(!v1.happens_before(&v2));
        assert!(!v2.happens_before(&v1));

        // Increment v1
        v1.increment("node1");
        assert!(!v1.happens_before(&v2));
        assert!(v2.happens_before(&v1));

        // Increment v2
        v2.increment("node2");
        assert!(v1.is_concurrent_with(&v2));

        // Merge
        v1.merge(&v2);
        assert_eq!(v1.counters.get("node1"), Some(&1));
        assert_eq!(v1.counters.get("node2"), Some(&1));
    }

    #[test]
    fn test_conflict_detection() {
        let mut v1 = VersionVector::new();
        let mut v2 = VersionVector::new();

        v1.increment("node1");
        v2.increment("node2");

        let e1 = VersionedEntry::new(
            "UPDATE users SET name = 'Alice' WHERE id = 1".to_string(),
            v1,
            ConflictStrategy::LastWriteWins,
            vec![],
        ).with_db_type(DatabaseType::PostgreSQL);

        let e2 = VersionedEntry::new(
            "UPDATE users SET name = 'Bob' WHERE id = 1".to_string(),
            v2,
            ConflictStrategy::LastWriteWins,
            vec![],
        ).with_db_type(DatabaseType::PostgreSQL);

        // Should detect conflict due to same row update
        assert!(e1.conflicts_with(&e2).unwrap());

        let e3 = VersionedEntry::new(
            "UPDATE users SET name = 'Charlie' WHERE id = 2".to_string(),
            v2,
            ConflictStrategy::LastWriteWins,
            vec![],
        ).with_db_type(DatabaseType::PostgreSQL);

        // Should not conflict due to different rows
        assert!(!e1.conflicts_with(&e3).unwrap());

        // Test DDL conflicts
        let e4 = VersionedEntry::new(
            "ALTER TABLE users ADD COLUMN age INTEGER".to_string(),
            v1,
            ConflictStrategy::LastWriteWins,
            vec![],
        ).with_db_type(DatabaseType::PostgreSQL);

        // DDL should conflict with DML
        assert!(e1.conflicts_with(&e4).unwrap());
    }

    #[test]
    fn test_conflict_resolution() {
        let mut v1 = VersionVector::new();
        let mut v2 = VersionVector::new();

        v1.increment("node1");
        v2.increment("node2");

        let e1 = VersionedEntry::new(
            "UPDATE users SET name = 'Alice' WHERE id = 1".to_string(),
            v1,
            ConflictStrategy::LastWriteWins,
            vec![],
        ).with_db_type(DatabaseType::PostgreSQL);

        // Create e2 with later timestamp
        tokio::time::sleep(std::time::Duration::from_millis(10));
        let e2 = VersionedEntry::new(
            "UPDATE users SET name = 'Bob' WHERE id = 1".to_string(),
            v2,
            ConflictStrategy::LastWriteWins,
            vec![],
        ).with_db_type(DatabaseType::PostgreSQL);

        // Last write wins should pick e2
        let resolved = e1.resolve_conflict(&e2).unwrap();
        assert_eq!(resolved.sql, e2.sql);

        // First write wins should pick e1
        let e1 = VersionedEntry::new(
            "UPDATE users SET name = 'Alice' WHERE id = 1".to_string(),
            v1,
            ConflictStrategy::FirstWriteWins,
            vec![],
        ).with_db_type(DatabaseType::PostgreSQL);

        let resolved = e1.resolve_conflict(&e2).unwrap();
        assert_eq!(resolved.sql, e1.sql);
    }
} 