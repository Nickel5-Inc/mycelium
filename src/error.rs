//! Error types for the Mycelium library.
//!
//! This module provides a comprehensive error handling system for network,
//! protocol, synchronization, and identity operations.

use std::io;
use thiserror::Error;

/// Primary error type encompassing all possible errors in the library.
#[derive(Error, Debug)]
pub enum Error {
    /// Protocol-related errors such as invalid messages or version mismatches
    #[error("Protocol error: {0}")]
    Protocol(String),

    /// Network operation errors including connection and transmission failures
    #[error("Network error: {0}")]
    Network(#[from] NetworkError),

    /// Data synchronization errors
    #[error("Sync error: {0}")]
    Sync(#[from] SyncError),

    /// Node identity and cryptographic operation errors
    #[error("Identity error: {0}")]
    Identity(#[from] IdentityError),

    /// Input/output operation errors
    #[error("IO error: {0}")]
    Io(#[from] io::Error),

    /// Message serialization/deserialization errors
    #[error("Serialization error: {0}")]
    Serialization(String),

    /// Configuration validation and parsing errors
    #[error("Configuration error: {0}")]
    Config(String),

    /// Internal library errors
    #[error("Internal error: {0}")]
    Internal(String),
}

/// Network-specific error types.
#[derive(Error, Debug)]
pub enum NetworkError {
    /// Failed to establish or maintain a connection
    #[error("Connection failed: {0}")]
    ConnectionFailed(String),

    /// Peer-specific errors
    #[error("Peer error: {0}")]
    PeerError(String),

    /// Transport layer errors
    #[error("Transport error: {0}")]
    TransportError(String),

    /// Network address parsing or binding errors
    #[error("Address error: {0}")]
    AddressError(String),

    /// TLS/encryption-related errors
    #[error("TLS error: {0}")]
    TlsError(String),
}

/// Synchronization-specific error types.
#[derive(Error, Debug)]
pub enum SyncError {
    /// Version conflicts between nodes
    #[error("Version mismatch: {0}")]
    VersionMismatch(String),

    /// Data conflicts during synchronization
    #[error("Conflict detected: {0}")]
    ConflictDetected(String),

    /// Storage operation errors
    #[error("Store error: {0}")]
    StoreError(String),

    /// Diff generation or application errors
    #[error("Diff error: {0}")]
    DiffError(String),
}

/// Identity and cryptographic operation error types.
#[derive(Error, Debug)]
pub enum IdentityError {
    /// Invalid key format or content
    #[error("Invalid key: {0}")]
    InvalidKey(String),

    /// Signature generation errors
    #[error("Signature error: {0}")]
    SignatureError(String),

    /// Signature verification failures
    #[error("Verification failed: {0}")]
    VerificationFailed(String),
}

/// Result type alias using our Error type
pub type Result<T> = std::result::Result<T, Error>;

impl Error {
    /// Creates a new protocol error with the given message.
    pub fn protocol(msg: impl Into<String>) -> Self {
        Error::Protocol(msg.into())
    }

    /// Creates a new network error with the given message.
    pub fn network(msg: impl Into<String>) -> Self {
        Error::Network(NetworkError::ConnectionFailed(msg.into()))
    }

    /// Creates a new sync error with the given message.
    pub fn sync(msg: impl Into<String>) -> Self {
        Error::Sync(SyncError::StoreError(msg.into()))
    }

    /// Creates a new identity error with the given message.
    pub fn identity(msg: impl Into<String>) -> Self {
        Error::Identity(IdentityError::InvalidKey(msg.into()))
    }

    /// Creates a new internal error with the given message.
    pub fn internal(msg: impl Into<String>) -> Self {
        Error::Internal(msg.into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_error_creation() {
        let err = Error::protocol("test error");
        assert!(matches!(err, Error::Protocol(_)));

        let err = Error::network("connection failed");
        assert!(matches!(err, Error::Network(_)));

        let err = Error::sync("store error");
        assert!(matches!(err, Error::Sync(_)));

        let err = Error::identity("invalid key");
        assert!(matches!(err, Error::Identity(_)));
    }

    #[test]
    fn test_error_conversion() {
        let io_err = io::Error::new(io::ErrorKind::Other, "io error");
        let err: Error = io_err.into();
        assert!(matches!(err, Error::Io(_)));
    }

    #[test]
    fn test_error_display() {
        let err = Error::protocol("test error");
        assert_eq!(err.to_string(), "Protocol error: test error");

        let err = Error::network("connection failed");
        assert_eq!(err.to_string(), "Network error: Connection failed: connection failed");
    }
} 