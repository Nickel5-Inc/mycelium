use std::collections::HashMap;
use std::sync::Arc;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use serde::{Serialize, Deserialize};

use super::{Database, Transaction, Error, Result};

/// Represents a database migration version
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
pub struct Version(pub u32);

/// Migration status
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum MigrationStatus {
    /// Migration is pending
    Pending,
    /// Migration has been applied
    Applied,
    /// Migration failed
    Failed,
}

/// Migration record stored in the database
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MigrationRecord {
    /// Migration version
    pub version: Version,
    /// Migration name
    pub name: String,
    /// When the migration was applied
    pub applied_at: DateTime<Utc>,
    /// Migration status
    pub status: MigrationStatus,
    /// Error message if failed
    pub error: Option<String>,
}

/// Migration trait that must be implemented by all migrations
#[async_trait]
pub trait Migration: Send + Sync {
    /// Get the migration version
    fn version(&self) -> Version;
    
    /// Get the migration name
    fn name(&self) -> &str;
    
    /// SQL to run during migration
    async fn up(&self, tx: &mut dyn Transaction) -> Result<()>;
    
    /// SQL to run during rollback
    async fn down(&self, tx: &mut dyn Transaction) -> Result<()>;
}

/// Registry of all available migrations
#[derive(Debug, Default)]
pub struct MigrationRegistry {
    migrations: HashMap<Version, Arc<dyn Migration>>,
}

impl MigrationRegistry {
    /// Create a new empty registry
    pub fn new() -> Self {
        Self {
            migrations: HashMap::new(),
        }
    }

    /// Register a new migration
    pub fn register(&mut self, migration: Arc<dyn Migration>) -> Result<()> {
        let version = migration.version();
        if self.migrations.contains_key(&version) {
            return Err(Error::Migration(format!(
                "Migration version {} already exists",
                version.0
            )));
        }
        self.migrations.insert(version, migration);
        Ok(())
    }

    /// Get all registered migrations sorted by version
    pub fn all(&self) -> Vec<Arc<dyn Migration>> {
        let mut migrations: Vec<_> = self.migrations.values().cloned().collect();
        migrations.sort_by_key(|m| m.version());
        migrations
    }

    /// Get a specific migration by version
    pub fn get(&self, version: Version) -> Option<Arc<dyn Migration>> {
        self.migrations.get(&version).cloned()
    }
}

/// Migration runner handles executing migrations
#[derive(Debug)]
pub struct MigrationRunner {
    registry: Arc<MigrationRegistry>,
}

impl MigrationRunner {
    /// Create a new migration runner
    pub fn new(registry: MigrationRegistry) -> Self {
        Self {
            registry: Arc::new(registry),
        }
    }

    /// Initialize migration tracking table
    pub async fn initialize(&self, db: &dyn Database) -> Result<()> {
        db.execute(
            "CREATE TABLE IF NOT EXISTS _migrations (
                version INTEGER PRIMARY KEY,
                name TEXT NOT NULL,
                applied_at TIMESTAMP NOT NULL,
                status TEXT NOT NULL,
                error TEXT
            )",
            &[],
        )
        .await?;
        Ok(())
    }

    /// Get all migration records
    pub async fn get_migrations(&self, db: &dyn Database) -> Result<Vec<MigrationRecord>> {
        let rows = db
            .query(
                "SELECT version, name, applied_at, status, error FROM _migrations ORDER BY version",
                &[],
            )
            .await?;

        let mut records = Vec::with_capacity(rows.len());
        for row in rows {
            records.push(MigrationRecord {
                version: Version(row.get("version")?),
                name: row.get("name")?,
                applied_at: row.get("applied_at")?,
                status: serde_json::from_str(&row.get::<String>("status")?)
                    .map_err(|e| Error::Migration(e.to_string()))?,
                error: row.get("error")?,
            });
        }
        Ok(records)
    }

    /// Run pending migrations
    pub async fn run_migrations(&self, db: &dyn Database) -> Result<()> {
        self.initialize(db).await?;
        
        let applied: HashMap<Version, MigrationRecord> = self
            .get_migrations(db)
            .await?
            .into_iter()
            .map(|r| (r.version, r))
            .collect();

        for migration in self.registry.all() {
            let version = migration.version();
            
            if let Some(record) = applied.get(&version) {
                if record.status == MigrationStatus::Failed {
                    // Retry failed migrations
                    self.run_migration(db, migration.clone()).await?;
                }
                continue;
            }

            self.run_migration(db, migration.clone()).await?;
        }

        Ok(())
    }

    /// Run a single migration within a transaction
    async fn run_migration(&self, db: &dyn Database, migration: Arc<dyn Migration>) -> Result<()> {
        let version = migration.version();
        let name = migration.name().to_string();

        let result = db
            .with_tx(|tx| {
                Box::pin(async move {
                    migration.up(tx).await?;
                    
                    tx.execute(
                        "INSERT INTO _migrations (version, name, applied_at, status, error) 
                         VALUES ($1, $2, $3, $4, $5)",
                        &[
                            &version.0,
                            &name,
                            &Utc::now(),
                            &serde_json::to_string(&MigrationStatus::Applied)
                                .map_err(|e| Error::Migration(e.to_string()))?,
                            &None::<String>,
                        ],
                    )
                    .await?;
                    
                    Ok(())
                })
            })
            .await;

        if let Err(e) = result {
            // Record failure
            db.execute(
                "INSERT INTO _migrations (version, name, applied_at, status, error) 
                 VALUES ($1, $2, $3, $4, $5)",
                &[
                    &version.0,
                    &name,
                    &Utc::now(),
                    &serde_json::to_string(&MigrationStatus::Failed)
                        .map_err(|e| Error::Migration(e.to_string()))?,
                    &Some(e.to_string()),
                ],
            )
            .await?;
            
            return Err(e);
        }

        Ok(())
    }

    /// Rollback migrations
    pub async fn rollback(&self, db: &dyn Database, target: Option<Version>) -> Result<()> {
        let applied = self.get_migrations(db).await?;
        
        let mut to_rollback: Vec<_> = applied
            .into_iter()
            .filter(|r| r.status == MigrationStatus::Applied)
            .collect();
        to_rollback.sort_by_key(|r| std::cmp::Reverse(r.version));

        for record in to_rollback {
            if let Some(target) = target {
                if record.version <= target {
                    break;
                }
            }

            if let Some(migration) = self.registry.get(record.version) {
                db.with_tx(|tx| {
                    Box::pin(async move {
                        migration.down(tx).await?;
                        
                        tx.execute(
                            "DELETE FROM _migrations WHERE version = $1",
                            &[&record.version.0],
                        )
                        .await?;
                        
                        Ok(())
                    })
                })
                .await?;
            }
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::database::{create_database, DatabaseConfig};
    use std::time::Duration;
    use tempfile::tempdir;

    struct TestMigration {
        version: Version,
        name: String,
    }

    #[async_trait]
    impl Migration for TestMigration {
        fn version(&self) -> Version {
            self.version
        }

        fn name(&self) -> &str {
            &self.name
        }

        async fn up(&self, tx: &mut dyn Transaction) -> Result<()> {
            tx.execute(
                &format!(
                    "CREATE TABLE test_{} (id INTEGER PRIMARY KEY)",
                    self.version.0
                ),
                &[],
            )
            .await?;
            Ok(())
        }

        async fn down(&self, tx: &mut dyn Transaction) -> Result<()> {
            tx.execute(
                &format!("DROP TABLE IF EXISTS test_{}", self.version.0),
                &[],
            )
            .await?;
            Ok(())
        }
    }

    #[tokio::test]
    async fn test_migration_registry() {
        let mut registry = MigrationRegistry::new();
        
        let m1 = TestMigration {
            version: Version(1),
            name: "test_1".into(),
        };
        registry.register(Arc::new(m1)).unwrap();

        let m2 = TestMigration {
            version: Version(2),
            name: "test_2".into(),
        };
        registry.register(Arc::new(m2)).unwrap();

        let migrations = registry.all();
        assert_eq!(migrations.len(), 2);
        assert_eq!(migrations[0].version(), Version(1));
        assert_eq!(migrations[1].version(), Version(2));
    }

    #[tokio::test]
    async fn test_migration_runner() {
        let dir = tempdir().unwrap();
        let db_path = dir.path().join("test.db");
        
        let config = DatabaseConfig {
            db_type: super::super::DatabaseType::SQLite,
            sqlite_path: Some(db_path.to_str().unwrap().to_string()),
            idle_timeout: Duration::from_secs(5),
            connect_timeout: Duration::from_secs(5),
            max_connections: 1,
            ..Default::default()
        };

        let db = create_database(config).await.unwrap();
        
        let mut registry = MigrationRegistry::new();
        
        let m1 = TestMigration {
            version: Version(1),
            name: "test_1".into(),
        };
        registry.register(Arc::new(m1)).unwrap();

        let m2 = TestMigration {
            version: Version(2),
            name: "test_2".into(),
        };
        registry.register(Arc::new(m2)).unwrap();

        let runner = MigrationRunner::new(registry);
        
        // Run migrations
        runner.run_migrations(&*db).await.unwrap();

        // Verify migrations table exists and has records
        let records = runner.get_migrations(&*db).await.unwrap();
        assert_eq!(records.len(), 2);
        assert_eq!(records[0].version, Version(1));
        assert_eq!(records[1].version, Version(2));
        assert_eq!(records[0].status, MigrationStatus::Applied);
        assert_eq!(records[1].status, MigrationStatus::Applied);

        // Verify test tables exist
        db.execute("SELECT 1 FROM test_1", &[]).await.unwrap();
        db.execute("SELECT 1 FROM test_2", &[]).await.unwrap();

        // Test rollback
        runner.rollback(&*db, Some(Version(1))).await.unwrap();

        // Verify test_2 table is dropped but test_1 remains
        db.execute("SELECT 1 FROM test_1", &[]).await.unwrap();
        assert!(db
            .execute("SELECT 1 FROM test_2", &[])
            .await
            .is_err());

        // Verify migration record is removed
        let records = runner.get_migrations(&*db).await.unwrap();
        assert_eq!(records.len(), 1);
        assert_eq!(records[0].version, Version(1));
    }
} 