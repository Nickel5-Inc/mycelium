use bytes::{Bytes, BytesMut};
use rmp_serde::{Deserializer, Serializer};
use serde::{Deserialize, Serialize};
use tokio_util::codec::{Decoder, Encoder};

use crate::error::{Error, Result};
use super::{Message, MAX_MESSAGE_SIZE};

/// Frame format:
/// +----------------+----------------+----------------+
/// |  Size (4B)    |  Flags (1B)    |  Payload      |
/// +----------------+----------------+----------------+
///
/// Size: Total frame size including header (u32, little endian)
/// Flags: Message flags (compression, encryption, etc.)
/// Payload: MessagePack encoded message

const HEADER_SIZE: usize = 5; // 4 bytes size + 1 byte flags
const FLAG_COMPRESSED: u8 = 0x01;
const FLAG_ENCRYPTED: u8 = 0x02;

/// Message codec for encoding/decoding protocol messages
#[derive(Debug, Clone)]
pub struct MessageCodec {
    max_message_size: usize,
}

impl Default for MessageCodec {
    fn default() -> Self {
        Self {
            max_message_size: MAX_MESSAGE_SIZE,
        }
    }
}

impl MessageCodec {
    /// Create a new message codec with custom maximum message size
    pub fn new(max_message_size: usize) -> Self {
        Self { max_message_size }
    }
}

impl Decoder for MessageCodec {
    type Item = Message;
    type Error = Error;

    fn decode(&mut self, src: &mut BytesMut) -> Result<Option<Self::Item>> {
        // Need at least the header to proceed
        if src.len() < HEADER_SIZE {
            return Ok(None);
        }

        // Read frame size
        let size = u32::from_le_bytes([src[0], src[1], src[2], src[3]]) as usize;
        if size > self.max_message_size {
            return Err(Error::protocol(format!(
                "message size {} exceeds maximum {}",
                size, self.max_message_size
            )));
        }

        // Check if we have the full frame
        if src.len() < size {
            // Need more data
            src.reserve(size - src.len());
            return Ok(None);
        }

        // Read flags
        let flags = src[4];

        // Extract payload
        let payload = src.split_to(size).freeze();
        let payload = &payload[HEADER_SIZE..];

        // Decompress if needed
        let payload = if flags & FLAG_COMPRESSED != 0 {
            // TODO: Implement decompression
            Bytes::from(payload.to_vec())
        } else {
            Bytes::from(payload.to_vec())
        };

        // Decrypt if needed
        let payload = if flags & FLAG_ENCRYPTED != 0 {
            // TODO: Implement decryption
            payload
        } else {
            payload
        };

        // Deserialize message
        let mut de = Deserializer::new(&payload[..]);
        let msg = Message::deserialize(&mut de).map_err(|e| {
            Error::protocol(format!("failed to deserialize message: {}", e))
        })?;

        Ok(Some(msg))
    }
}

impl Encoder<Message> for MessageCodec {
    type Error = Error;

    fn encode(&mut self, msg: Message, dst: &mut BytesMut) -> Result<()> {
        // Reserve space for header
        dst.reserve(HEADER_SIZE);
        let start = dst.len();
        dst.extend_from_slice(&[0u8; HEADER_SIZE]);

        // Initialize flags
        let mut flags = 0u8;

        // Serialize message
        let mut buf = Vec::new();
        msg.serialize(&mut Serializer::new(&mut buf)).map_err(|e| {
            Error::protocol(format!("failed to serialize message: {}", e))
        })?;

        // Compress if beneficial
        let payload = if buf.len() > 512 {
            // TODO: Implement compression
            flags |= FLAG_COMPRESSED;
            buf
        } else {
            buf
        };

        // Encrypt if needed
        let payload = if false {
            // TODO: Implement encryption
            flags |= FLAG_ENCRYPTED;
            payload
        } else {
            payload
        };

        // Write payload
        dst.extend_from_slice(&payload);

        // Write header
        let frame_size = dst.len() - start;
        if frame_size > self.max_message_size {
            return Err(Error::protocol(format!(
                "message size {} exceeds maximum {}",
                frame_size, self.max_message_size
            )));
        }

        let size_bytes = (frame_size as u32).to_le_bytes();
        dst[start..start + 4].copy_from_slice(&size_bytes);
        dst[start + 4] = flags;

        Ok(())
    }
}

/// Message encoder for async streams
pub type MessageEncoder = tokio_util::codec::FramedWrite<tokio::io::WriteHalf<tokio::net::TcpStream>, MessageCodec>;

/// Message decoder for async streams
pub type MessageDecoder = tokio_util::codec::FramedRead<tokio::io::ReadHalf<tokio::net::TcpStream>, MessageCodec>;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::protocol::NodeId;
    use super::super::MessageType;

    #[test]
    fn test_codec_roundtrip() {
        let mut codec = MessageCodec::default();
        let mut buf = BytesMut::new();

        // Create test message
        let node_id = NodeId::new("test-node");
        let payload = b"test payload".to_vec();
        let msg = Message::new(MessageType::Data, node_id, payload);

        // Encode
        codec.encode(msg.clone(), &mut buf).unwrap();

        // Decode
        let decoded = codec.decode(&mut buf).unwrap().unwrap();

        // Verify
        assert_eq!(decoded.msg_type, msg.msg_type);
        assert_eq!(decoded.sender, msg.sender);
        assert_eq!(decoded.payload, msg.payload);
    }

    #[test]
    fn test_codec_size_limit() {
        let mut codec = MessageCodec::new(100);
        let mut buf = BytesMut::new();

        // Create message that exceeds size limit
        let node_id = NodeId::new("test-node");
        let payload = vec![0u8; 1000];
        let msg = Message::new(MessageType::Data, node_id, payload);

        // Encoding should fail
        assert!(codec.encode(msg, &mut buf).is_err());
    }

    #[test]
    fn test_codec_partial_decode() {
        let mut codec = MessageCodec::default();
        let mut buf = BytesMut::new();

        // Create test message
        let node_id = NodeId::new("test-node");
        let payload = b"test payload".to_vec();
        let msg = Message::new(MessageType::Data, node_id, payload);

        // Encode
        codec.encode(msg, &mut buf).unwrap();

        // Split buffer and try to decode
        let partial = buf.split_to(5);
        assert!(codec.decode(&mut partial.clone()).unwrap().is_none());
    }
} 