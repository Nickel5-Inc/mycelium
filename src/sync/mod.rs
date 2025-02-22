use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{mpsc, RwLock};
use std::fmt::Debug;
use tokio::io::{AsyncRead, AsyncWrite};

use crate::error::{Error, Result};
use crate::network::{Network, PeerEvent, Transport, TcpTransport};
use crate::protocol::{Message, MessageType, NodeId};

/// Database sync state
pub struct SyncState<T: Transport> {
    /// Network instance
    network: Arc<RwLock<Network<T>>>,
    /// Pending sync requests
    pending_requests: HashMap<NodeId, mpsc::Sender<Message>>,
}

impl<T> SyncState<T> 
where 
    T: Transport + Send + Sync + 'static,
    T::Stream: AsyncRead + AsyncWrite + Unpin + Send + Sync + Debug + 'static,
{
    /// Create a new sync state
    pub fn new(network: Arc<RwLock<Network<T>>>) -> Self {
        Self {
            network,
            pending_requests: HashMap::new(),
        }
    }

    /// Start sync process
    pub async fn start(&mut self) -> Result<()> {
        let network = self.network.clone();
        let mut events_rx = {
            let network = network.write().await;
            network.events()
        };

        // Process network events
        while let Ok(event) = events_rx.recv().await {
            match event {
                PeerEvent::Message(msg) => {
                    match msg.msg_type {
                        MessageType::DBSyncReq => {
                            self.handle_sync_request(msg).await?;
                        }
                        MessageType::DBSyncResp => {
                            self.handle_sync_response(msg).await?;
                        }
                        _ => {}
                    }
                }
                PeerEvent::Disconnected => {
                    // No need to remove anything here since we don't have the peer ID
                }
                PeerEvent::Error(_) => {}
            }
        }

        Ok(())
    }

    /// Request database sync from a peer
    pub async fn request_sync(&mut self, peer_id: &NodeId, data: Vec<u8>) -> Result<()> {
        // Create response channel
        let (tx, mut rx) = mpsc::channel(1);
        self.pending_requests.insert(peer_id.clone(), tx);

        // Send sync request
        let msg = Message::new(
            MessageType::DBSyncReq,
            peer_id.clone(),
            data,
        );

        let network = self.network.read().await;
        network.broadcast(msg).await?;

        // Wait for response with timeout
        match tokio::time::timeout(
            std::time::Duration::from_secs(30),
            rx.recv()
        ).await {
            Ok(Some(resp)) => {
                println!("Received sync response from {}", peer_id);
                self.process_sync_data(resp.payload.as_ref().to_vec()).await?;
            }
            Ok(None) => {
                return Err(Error::sync("sync response channel closed"));
            }
            Err(_) => {
                return Err(Error::sync("sync response timeout"));
            }
        }

        Ok(())
    }

    /// Handle incoming sync request
    async fn handle_sync_request(&mut self, msg: Message) -> Result<()> {
        println!("Received sync request from {}", msg.sender);

        // Get sync data
        let data = self.get_sync_data().await?;

        // Send response
        let resp = Message::new(
            MessageType::DBSyncResp,
            msg.sender.clone(),
            data,
        );

        let network = self.network.read().await;
        network.broadcast(resp).await?;

        Ok(())
    }

    /// Handle incoming sync response
    async fn handle_sync_response(&mut self, msg: Message) -> Result<()> {
        println!("Received sync response from {}", msg.sender);

        // Forward response to waiting request
        if let Some(tx) = self.pending_requests.remove(&msg.sender) {
            tx.send(msg).await.map_err(|_| {
                Error::sync("failed to forward sync response")
            })?;
        }

        Ok(())
    }

    /// Get database sync data
    async fn get_sync_data(&self) -> Result<Vec<u8>> {
        // TODO: Implement actual database sync logic
        Ok(vec![])
    }

    /// Process received sync data
    async fn process_sync_data(&self, data: Vec<u8>) -> Result<()> {
        // TODO: Implement actual database sync logic
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::network::{NetworkConfig, TcpTransport};
    use tokio::time::sleep;
    use std::time::Duration;

    #[tokio::test]
    async fn test_sync() {
        // Create networks
        let mut config1 = NetworkConfig::default();
        config1.local_addr = "127.0.0.1:0".parse().unwrap();
        let network1 = Arc::new(RwLock::new(Network::new(config1.clone(), TcpTransport)));

        let mut config2 = NetworkConfig::default();
        config2.local_addr = "127.0.0.1:0".parse().unwrap();
        let network2 = Arc::new(RwLock::new(Network::new(config2, TcpTransport)));

        // Start networks
        let network1_clone = network1.clone();
        let network1_handle = tokio::spawn(async move {
            let mut network = network1_clone.write().await;
            network.start().await.unwrap();
        });

        let network2_clone = network2.clone();
        let network2_handle = tokio::spawn(async move {
            let mut network = network2_clone.write().await;
            network.start().await.unwrap();
        });

        // Wait for networks to start
        sleep(Duration::from_millis(100)).await;

        // Connect networks
        {
            let network1 = network1.write().await;
            network1.connect(config1.local_addr).await.unwrap();
        }

        // Create sync states
        let mut sync1 = SyncState::new(network1);
        let mut sync2 = SyncState::new(network2);

        // Start sync processes
        let sync1_handle = tokio::spawn(async move {
            sync1.start().await.unwrap();
        });

        let sync2_handle = tokio::spawn(async move {
            sync2.start().await.unwrap();
        });

        // Wait for sync
        sleep(Duration::from_secs(1)).await;

        // Cleanup
        network1_handle.abort();
        network2_handle.abort();
        sync1_handle.abort();
        sync2_handle.abort();
    }
} 