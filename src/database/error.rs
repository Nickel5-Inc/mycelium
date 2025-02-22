use std::error::Error as StdError;
use std::fmt;

/// Database error type
#[derive(Debug)]
pub enum Error {
    /// Connection error
    Connection(String),
    /// Query error
    Query(String),
    /// Transaction error
    Transaction(String),
    /// Type mismatch error
    TypeMismatch,
    /// Column not found error
    ColumnNotFound(String),
    /// Migration error
    Migration(String),
    /// Configuration error
    Config(String),
    /// Replication error
    Replication(String),
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Error::Connection(msg) => write!(f, "Connection error: {}", msg),
            Error::Query(msg) => write!(f, "Query error: {}", msg),
            Error::Transaction(msg) => write!(f, "Transaction error: {}", msg),
            Error::TypeMismatch => write!(f, "Type mismatch error"),
            Error::ColumnNotFound(col) => write!(f, "Column not found: {}", col),
            Error::Migration(msg) => write!(f, "Migration error: {}", msg),
            Error::Config(msg) => write!(f, "Configuration error: {}", msg),
            Error::Replication(msg) => write!(f, "Replication error: {}", msg),
        }
    }
}

impl StdError for Error {}

/// Result type for database operations
pub type Result<T> = std::result::Result<T, Error>; 