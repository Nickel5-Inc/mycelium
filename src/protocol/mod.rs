use serde::{Deserialize, Serialize};
use std::fmt;

/// Message types and core message handling
mod message;
/// Message encoding and decoding
pub mod codec;
/// Byte serialization utilities
mod bytes;

pub use message::{Message, MessageType};
pub use codec::{MessageCodec, MessageDecoder, MessageEncoder};
pub use bytes::SerializedBytes;

/// Protocol version for compatibility checking
pub const PROTOCOL_VERSION: &str = "1.0.0";

/// Minimum supported protocol version
pub const MIN_PROTOCOL_VERSION: &str = "1.0.0";

/// Maximum message size in bytes
pub const MAX_MESSAGE_SIZE: usize = 16 * 1024 * 1024; // 16MB

/// Node ID type
#[derive(Debug, Clone, Hash, Eq, PartialEq, Serialize, Deserialize)]
pub struct NodeId(String);

impl NodeId {
    /// Create a new NodeId from a string
    pub fn new(id: impl Into<String>) -> Self {
        Self(id.into())
    }

    /// Get the string representation of the NodeId
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl fmt::Display for NodeId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

/// Protocol capabilities and features
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Capabilities {
    /// Protocol version
    pub version: String,
    /// Supported message types
    pub message_types: Vec<MessageType>,
    /// Maximum message size
    pub max_message_size: usize,
    /// Supported compression algorithms
    pub compression: Vec<String>,
    /// Supported encryption algorithms
    pub encryption: Vec<String>,
}

impl Default for Capabilities {
    fn default() -> Self {
        Self {
            version: PROTOCOL_VERSION.to_string(),
            message_types: MessageType::all(),
            max_message_size: MAX_MESSAGE_SIZE,
            compression: vec!["none".to_string()],
            encryption: vec!["none".to_string()],
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_node_id() {
        let id = NodeId::new("test-node");
        assert_eq!(id.as_str(), "test-node");
        assert_eq!(id.to_string(), "test-node");
    }

    #[test]
    fn test_capabilities() {
        let caps = Capabilities::default();
        assert_eq!(caps.version, PROTOCOL_VERSION);
        assert!(!caps.message_types.is_empty());
        assert_eq!(caps.max_message_size, MAX_MESSAGE_SIZE);
    }
} 