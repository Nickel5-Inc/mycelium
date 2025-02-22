//! Mycelium is a lightweight and performant mesh networking library for validator nodes.
//!
//! This library provides a flexible and efficient way to create mesh networks between
//! validator nodes, with support for real-time data synchronization and customizable
//! schemas.

#![warn(missing_docs)]
#![warn(rust_2018_idioms)]

pub mod error;
/// Network layer handling peer connections and message routing
pub mod network;
/// Protocol definitions and message handling
pub mod protocol;
/// Data synchronization and state management
pub mod sync;
/// Node identity and cryptographic operations
pub mod identity;

// Re-export common types
pub use error::{Error, Result};
pub use network::{Network, NetworkConfig, Peer, PeerEvent};
pub use protocol::{Message, MessageType, NodeId};
pub use sync::SyncState;
pub use identity::Identity;

/// Version of the Mycelium library
pub const VERSION: &str = env!("CARGO_PKG_VERSION");

/// Feature flags
pub mod features {
    /// Whether TCP transport is enabled
    pub const TCP_TRANSPORT: bool = cfg!(feature = "transport-tcp");
    /// Whether QUIC transport is enabled
    pub const QUIC_TRANSPORT: bool = cfg!(feature = "transport-quic");
    /// Whether metrics are enabled
    pub const METRICS: bool = cfg!(feature = "metrics");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_version() {
        assert!(!VERSION.is_empty());
    }

    #[test]
    fn test_features() {
        // At least one transport should be enabled
        assert!(features::TCP_TRANSPORT || features::QUIC_TRANSPORT);
    }
}
