use std::net::SocketAddr;
use std::time::{Duration, Instant};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::broadcast;
use tokio_util::codec::Framed;
use futures::{SinkExt, StreamExt};

use crate::error::{Error, Result};
use crate::protocol::{Message, MessageCodec, NodeId};

/// Peer events
#[derive(Debug, Clone)]
pub enum PeerEvent {
    /// Message received from peer
    Message(Message),
    /// Peer disconnected
    Disconnected,
    /// Error occurred
    Error(Error),
}

impl Clone for Error {
    fn clone(&self) -> Self {
        Error::internal(self.to_string())
    }
}

/// Peer connection state
#[derive(Debug)]
pub struct Peer<S> {
    /// Peer ID
    id: NodeId,
    /// Peer address
    addr: SocketAddr,
    /// Connection frame
    frame: Framed<S, MessageCodec>,
    /// Event sender
    event_tx: broadcast::Sender<PeerEvent>,
    /// Last seen timestamp
    last_seen: Instant,
    /// Bytes sent
    bytes_sent: u64,
    /// Bytes received
    bytes_received: u64,
}

impl<S> Peer<S>
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    /// Create a new peer
    pub fn new(
        id: NodeId,
        addr: SocketAddr,
        stream: S,
        event_tx: broadcast::Sender<PeerEvent>,
    ) -> Self {
        Self {
            id,
            addr,
            frame: Framed::new(stream, MessageCodec::default()),
            event_tx,
            last_seen: Instant::now(),
            bytes_sent: 0,
            bytes_received: 0,
        }
    }

    /// Get peer ID
    pub fn id(&self) -> &NodeId {
        &self.id
    }

    /// Get peer address
    pub fn addr(&self) -> SocketAddr {
        self.addr
    }

    /// Get last seen timestamp
    pub fn last_seen(&self) -> Instant {
        self.last_seen
    }

    /// Get bytes sent
    pub fn bytes_sent(&self) -> u64 {
        self.bytes_sent
    }

    /// Get bytes received
    pub fn bytes_received(&self) -> u64 {
        self.bytes_received
    }

    /// Send a message to the peer
    pub async fn send_message(&mut self, message: Message) -> Result<()> {
        self.frame.send(message).await.map_err(|e| {
            Error::network(format!("failed to send message to peer {}: {}", self.id, e))
        })?;
        Ok(())
    }

    /// Start processing messages from the peer
    pub async fn start(&mut self) -> Result<()> {
        // Process incoming messages
        while let Some(result) = self.frame.next().await {
            match result {
                Ok(msg) => {
                    self.last_seen = Instant::now();
                    self.bytes_received += msg.size() as u64;

                    // Forward message to event handler
                    if let Err(e) = self.event_tx.send(PeerEvent::Message(msg)) {
                        return Err(Error::network(format!(
                            "failed to send peer event: {}",
                            e
                        )));
                    }
                }
                Err(e) => {
                    let err = Error::network(format!(
                        "error receiving message from peer {}: {}",
                        self.id, e
                    ));
                    if let Err(e) = self.event_tx.send(PeerEvent::Error(err.clone())) {
                        return Err(Error::network(format!(
                            "failed to send peer error event: {}",
                            e
                        )));
                    }
                    return Err(err);
                }
            }
        }

        // Peer disconnected
        if let Err(e) = self.event_tx.send(PeerEvent::Disconnected) {
            return Err(Error::network(format!(
                "failed to send peer disconnected event: {}",
                e
            )));
        }

        Ok(())
    }

    /// Check if the peer is alive based on last seen timestamp
    pub fn is_alive(&self, timeout: Duration) -> bool {
        self.last_seen.elapsed() < timeout
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::net::TcpStream;

    #[tokio::test]
    async fn test_peer_connection() {
        // Create event channel
        let (tx, _rx) = broadcast::channel(10);

        // Create peer
        let addr = "127.0.0.1:0".parse().unwrap();
        let stream = TcpStream::connect(addr).await.unwrap();
        let node_id = NodeId::new("test-node");
        let peer = Peer::new(node_id.clone(), addr, stream, tx);

        // Verify peer info
        assert_eq!(peer.id(), &node_id);
        assert_eq!(peer.addr(), addr);
        assert!(peer.is_alive(Duration::from_secs(1)));
    }
} 