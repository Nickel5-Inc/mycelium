use std::time::Duration;
use super::DatabaseType;

/// Database configuration
#[derive(Debug, Clone)]
pub struct DatabaseConfig {
    /// Database type (SQLite or PostgreSQL)
    pub db_type: DatabaseType,
    
    /// Database host (PostgreSQL only)
    pub host: Option<String>,
    
    /// Database port (PostgreSQL only)
    pub port: Option<u16>,
    
    /// Database user (PostgreSQL only)
    pub user: Option<String>,
    
    /// Database password (PostgreSQL only)
    pub password: Option<String>,
    
    /// Database name
    pub database: String,
    
    /// SQLite file path (SQLite only)
    pub sqlite_path: Option<String>,
    
    /// Maximum number of connections in the pool
    pub max_connections: u32,
    
    /// Connection idle timeout
    pub idle_timeout: Duration,
    
    /// Connection timeout
    pub connect_timeout: Duration,
    
    /// SSL mode (PostgreSQL only)
    pub ssl_mode: Option<String>,
}

impl Default for DatabaseConfig {
    fn default() -> Self {
        Self {
            db_type: DatabaseType::default(),
            host: None,
            port: None,
            user: None,
            password: None,
            database: String::from("mycelium"),
            sqlite_path: Some(String::from("mycelium.db")),
            max_connections: 10,
            idle_timeout: Duration::from_secs(300),
            connect_timeout: Duration::from_secs(10),
            ssl_mode: None,
        }
    }
}

impl DatabaseConfig {
    /// Creates a new SQLite configuration
    pub fn new_sqlite(path: impl Into<String>) -> Self {
        Self {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(path.into()),
            ..Default::default()
        }
    }

    /// Creates a new PostgreSQL configuration
    pub fn new_postgres(
        host: impl Into<String>,
        port: u16,
        user: impl Into<String>,
        password: impl Into<String>,
        database: impl Into<String>,
    ) -> Self {
        Self {
            db_type: DatabaseType::PostgreSQL,
            host: Some(host.into()),
            port: Some(port),
            user: Some(user.into()),
            password: Some(password.into()),
            database: database.into(),
            sqlite_path: None,
            ..Default::default()
        }
    }

    /// Validates the configuration
    pub fn validate(&self) -> super::Result<()> {
        match self.db_type {
            DatabaseType::SQLite => {
                if self.sqlite_path.is_none() {
                    return Err(super::Error::Config("SQLite path is required".into()));
                }
            }
            DatabaseType::PostgreSQL => {
                if self.host.is_none() {
                    return Err(super::Error::Config("PostgreSQL host is required".into()));
                }
                if self.port.is_none() {
                    return Err(super::Error::Config("PostgreSQL port is required".into()));
                }
                if self.user.is_none() {
                    return Err(super::Error::Config("PostgreSQL user is required".into()));
                }
                if self.password.is_none() {
                    return Err(super::Error::Config("PostgreSQL password is required".into()));
                }
            }
        }
        Ok(())
    }
} 