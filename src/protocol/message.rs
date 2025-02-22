use bytes::Bytes;
use serde::{Deserialize, Serialize};
use std::time::{SystemTime, UNIX_EPOCH};
use uuid::Uuid;

use crate::error::Result;
use super::{NodeId, SerializedBytes};

/// Message types supported by the protocol
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[repr(u8)]
pub enum MessageType {
    /// Handshake message for establishing connections
    Handshake = 0,
    /// Handshake response
    HandshakeAck = 1,
    /// Ping message for connection keepalive
    Ping = 2,
    /// Pong response to ping
    Pong = 3,
    /// Peer discovery message
    Discovery = 4,
    /// Data sync request
    SyncRequest = 5,
    /// Data sync response
    SyncResponse = 6,
    /// Data update notification
    Update = 7,
    /// Generic data message
    Data = 8,
    /// Database sync request
    DBSyncReq = 9,
    /// Database sync response
    DBSyncResp = 10,
}

impl MessageType {
    /// Get all supported message types
    pub fn all() -> Vec<MessageType> {
        vec![
            MessageType::Handshake,
            MessageType::HandshakeAck,
            MessageType::Ping,
            MessageType::Pong,
            MessageType::Discovery,
            MessageType::SyncRequest,
            MessageType::SyncResponse,
            MessageType::Update,
            MessageType::Data,
            MessageType::DBSyncReq,
            MessageType::DBSyncResp,
        ]
    }
}

/// Core message structure
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    /// Unique message identifier
    pub id: Uuid,
    /// Message type
    pub msg_type: MessageType,
    /// Sender node ID
    pub sender: NodeId,
    /// Timestamp in milliseconds since UNIX epoch
    pub timestamp: u64,
    /// Message payload
    pub payload: SerializedBytes,
    /// Optional message signature
    #[serde(skip_serializing_if = "Option::is_none")]
    pub signature: Option<SerializedBytes>,
}

impl Message {
    /// Create a new message
    pub fn new(msg_type: MessageType, sender: NodeId, payload: impl Into<Bytes>) -> Self {
        Self {
            id: Uuid::new_v4(),
            msg_type,
            sender,
            timestamp: SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_millis() as u64,
            payload: SerializedBytes::new(payload),
            signature: None,
        }
    }

    /// Sign the message with the provided key
    pub fn sign(&mut self, _key: &[u8]) -> Result<()> {
        // TODO: Implement message signing
        Ok(())
    }

    /// Verify the message signature
    pub fn verify(&self, _public_key: &[u8]) -> Result<bool> {
        // TODO: Implement signature verification
        Ok(true)
    }

    /// Get the message size in bytes
    pub fn size(&self) -> usize {
        16 + // UUID
        1 +  // message type
        self.sender.as_str().len() +
        8 +  // timestamp
        self.payload.len() +
        self.signature.as_ref().map_or(0, |s| s.len())
    }
}

/// Handshake message payload
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Handshake {
    /// Protocol version
    pub version: String,
    /// Node capabilities
    pub capabilities: super::Capabilities,
    /// Node public key
    pub public_key: SerializedBytes,
}

/// Sync request payload
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncRequest {
    /// Last known version
    pub version: u64,
    /// Maximum number of items to sync
    pub limit: usize,
    /// Filter criteria
    pub filter: Option<String>,
}

/// Sync response payload
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncResponse {
    /// Current version
    pub version: u64,
    /// Sync data
    pub data: Vec<SyncData>,
    /// Whether there is more data available
    pub has_more: bool,
}

/// Sync data item
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncData {
    /// Data ID
    pub id: String,
    /// Version number
    pub version: u64,
    /// Data payload
    pub data: SerializedBytes,
    /// Timestamp
    pub timestamp: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_message_creation() {
        let node_id = NodeId::new("test-node");
        let payload = b"test payload".to_vec();
        let msg = Message::new(MessageType::Data, node_id.clone(), payload);

        assert_eq!(msg.msg_type, MessageType::Data);
        assert_eq!(msg.sender, node_id);
        assert!(!msg.payload.is_empty());
        assert!(msg.signature.is_none());
    }

    #[test]
    fn test_message_size() {
        let node_id = NodeId::new("test-node");
        let payload = b"test payload".to_vec();
        let msg = Message::new(MessageType::Data, node_id, payload);

        let expected_size = 16 + 1 + 9 + 8 + 11 + 0; // UUID + type + sender + timestamp + payload + signature
        assert_eq!(msg.size(), expected_size);
    }

    #[test]
    fn test_message_types() {
        let types = MessageType::all();
        assert_eq!(types.len(), 12);
        assert!(types.contains(&MessageType::Handshake));
        assert!(types.contains(&MessageType::Data));
    }
} 