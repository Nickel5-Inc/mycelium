use std::fmt::Debug;
use std::sync::Arc;
use tokio::sync::Mutex;
use async_trait::async_trait;

pub mod error;
pub mod postgres;
pub mod sqlite;
pub mod config;
pub mod migrations;
pub mod query;
pub mod replication;
pub mod pool;

pub use error::{Error, Result};
pub use config::DatabaseConfig;
pub use migrations::{Migration, MigrationRegistry, MigrationRunner, Version};
pub use query::{QueryBuilder, Condition, Operator, JoinType, OrderDirection, Join};
pub use replication::{ReplicationManager, ReplicationState, NodeRole, WalEntry};
pub use pool::{ConnectionPool, PoolConfig, PoolStats};

/// Database driver type
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum DatabaseType {
    /// SQLite database
    SQLite,
    /// PostgreSQL database
    PostgreSQL,
}

impl Default for DatabaseType {
    fn default() -> Self {
        DatabaseType::SQLite
    }
}

/// Transaction trait for database operations
#[async_trait]
pub trait Transaction: Send + Sync {
    /// Commits the transaction
    async fn commit(&mut self) -> Result<()>;
    
    /// Rolls back the transaction
    async fn rollback(&mut self) -> Result<()>;
    
    /// Executes a query that returns no rows
    async fn execute(&mut self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<u64>;
    
    /// Executes a query that returns rows
    async fn query(&mut self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<Vec<Row>>;
}

/// Database trait for common operations
#[async_trait]
pub trait Database: Send + Sync + Debug {
    /// The transaction type for this database
    type Tx: Transaction;

    /// Connects to the database
    async fn connect(config: DatabaseConfig) -> Result<Self> where Self: Sized;
    
    /// Pings the database to check connectivity
    async fn ping(&self) -> Result<()>;
    
    /// Closes the database connection
    async fn close(&self) -> Result<()>;
    
    /// Begins a new transaction
    async fn begin_tx(&self) -> Result<Self::Tx>;
    
    /// Executes a function within a transaction
    async fn with_tx<F, T>(&self, f: F) -> Result<T>
    where
        F: FnOnce(&mut Self::Tx) -> Result<T> + Send + 'static,
        T: Send + 'static;
        
    /// Executes a query that returns no rows
    async fn execute(&self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<u64>;
    
    /// Executes a query that returns rows
    async fn query(&self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<Vec<Row>>;
}

/// Row represents a database row
#[derive(Debug, Clone)]
pub struct Row {
    columns: Vec<String>,
    values: Vec<Value>,
}

impl Row {
    /// Gets a column value by name
    pub fn get<T: FromValue>(&self, column: &str) -> Result<T> {
        let idx = self.columns.iter()
            .position(|c| c == column)
            .ok_or_else(|| Error::ColumnNotFound(column.to_string()))?;
        T::from_value(&self.values[idx])
    }
}

/// Value represents a database value
#[derive(Debug, Clone)]
pub enum Value {
    Null,
    Bool(bool),
    Int(i64),
    Float(f64),
    Text(String),
    Bytes(Vec<u8>),
    // Add more types as needed
}

/// Trait for converting Rust types to SQL values
pub trait ToSql {
    fn to_sql(&self) -> Result<Value>;
}

/// Trait for converting SQL values to Rust types
pub trait FromValue: Sized {
    fn from_value(value: &Value) -> Result<Self>;
}

// Common implementations
impl ToSql for i32 {
    fn to_sql(&self) -> Result<Value> {
        Ok(Value::Int(*self as i64))
    }
}

impl ToSql for i64 {
    fn to_sql(&self) -> Result<Value> {
        Ok(Value::Int(*self))
    }
}

impl ToSql for f64 {
    fn to_sql(&self) -> Result<Value> {
        Ok(Value::Float(*self))
    }
}

impl ToSql for &str {
    fn to_sql(&self) -> Result<Value> {
        Ok(Value::Text(self.to_string()))
    }
}

impl ToSql for String {
    fn to_sql(&self) -> Result<Value> {
        Ok(Value::Text(self.clone()))
    }
}

impl FromValue for i32 {
    fn from_value(value: &Value) -> Result<Self> {
        match value {
            Value::Int(i) => Ok(*i as i32),
            _ => Err(Error::TypeMismatch),
        }
    }
}

impl FromValue for i64 {
    fn from_value(value: &Value) -> Result<Self> {
        match value {
            Value::Int(i) => Ok(*i),
            _ => Err(Error::TypeMismatch),
        }
    }
}

impl FromValue for f64 {
    fn from_value(value: &Value) -> Result<Self> {
        match value {
            Value::Float(f) => Ok(*f),
            _ => Err(Error::TypeMismatch),
        }
    }
}

impl FromValue for String {
    fn from_value(value: &Value) -> Result<Self> {
        match value {
            Value::Text(s) => Ok(s.clone()),
            _ => Err(Error::TypeMismatch),
        }
    }
}

/// Creates a new database instance based on configuration
pub async fn create_database(config: DatabaseConfig) -> Result<Box<dyn Database>> {
    config.validate()?;
    
    match config.db_type {
        DatabaseType::SQLite => {
            let db = sqlite::SQLiteDatabase::connect(config).await?;
            Ok(Box::new(db))
        }
        DatabaseType::PostgreSQL => {
            let db = postgres::PostgresDatabase::connect(config).await?;
            Ok(Box::new(db))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_sqlite_database() {
        let dir = tempdir().unwrap();
        let db_path = dir.path().join("test.db");
        
        let config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(db_path.to_str().unwrap().to_string()),
            database: "test".to_string(),
            ..Default::default()
        };

        let db = create_database(config).await.unwrap();
        
        // Test basic operations
        db.execute(
            "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)",
            &[],
        ).await.unwrap();

        let name = "test";
        db.execute(
            "INSERT INTO test (name) VALUES (?)",
            &[&name],
        ).await.unwrap();

        let rows = db.query(
            "SELECT name FROM test WHERE id = 1",
            &[],
        ).await.unwrap();

        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].get::<String>("name").unwrap(), "test");
    }

    #[tokio::test]
    async fn test_postgres_database() {
        let config = DatabaseConfig {
            db_type: DatabaseType::PostgreSQL,
            host: Some("localhost".to_string()),
            port: Some(5432),
            user: Some("postgres".to_string()),
            password: Some("postgres".to_string()),
            database: "postgres".to_string(),
            max_connections: 5,
            idle_timeout: Duration::from_secs(5),
            ..Default::default()
        };

        let db = create_database(config).await.unwrap();
        
        // Test basic operations
        db.execute(
            "CREATE TABLE IF NOT EXISTS test (id SERIAL PRIMARY KEY, name TEXT)",
            &[],
        ).await.unwrap();

        let name = "test";
        db.execute(
            "INSERT INTO test (name) VALUES ($1)",
            &[&name],
        ).await.unwrap();

        let rows = db.query(
            "SELECT name FROM test WHERE id = 1",
            &[],
        ).await.unwrap();

        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].get::<String>("name").unwrap(), "test");

        // Clean up
        db.execute("DROP TABLE test", &[]).await.unwrap();
    }
} 