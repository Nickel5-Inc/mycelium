//! Network module providing peer-to-peer communication functionality.
//!
//! This module handles peer connections, message broadcasting, and network event management.

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::RwLock;
use tokio::time;
use tokio::sync::broadcast;
use tokio::io::{AsyncRead, AsyncWrite};

use crate::error::{Error, Result};
use crate::protocol::{Message, NodeId};

mod peer;
mod transport;
mod cert;

pub use peer::{Peer, PeerEvent};
pub use transport::{Transport, TcpTransport, TlsTransport};
pub use cert::{CertificateConfig, CertificateManager};

/// Configuration options for the network layer.
#[derive(Debug, Clone)]
pub struct NetworkConfig {
    /// Local address to bind to for accepting incoming connections
    pub local_addr: SocketAddr,
    /// Maximum number of concurrent peer connections allowed
    pub max_connections: usize,
    /// Timeout duration for establishing new connections
    pub connection_timeout: Duration,
    /// Interval between keep-alive messages to maintain connections
    pub keep_alive_interval: Duration,
    /// TLS certificate file path (optional)
    pub tls_cert_path: Option<String>,
    /// TLS private key file path (optional)
    pub tls_key_path: Option<String>,
    /// Whether to require TLS for connections
    pub require_tls: bool,
}

impl Default for NetworkConfig {
    /// Creates a default network configuration.
    ///
    /// Default values:
    /// - Local address: 127.0.0.1:0 (random port)
    /// - Max connections: 50
    /// - Connection timeout: 10 seconds
    /// - Keep-alive interval: 30 seconds
    /// - TLS: disabled
    fn default() -> Self {
        Self {
            local_addr: "127.0.0.1:0".parse().unwrap(),
            max_connections: 50,
            connection_timeout: Duration::from_secs(10),
            keep_alive_interval: Duration::from_secs(30),
            tls_cert_path: None,
            tls_key_path: None,
            require_tls: false,
        }
    }
}

/// Core network state managing peer connections and message routing.
#[derive(Debug)]
pub struct Network<T: Transport> {
    /// Network configuration
    config: NetworkConfig,
    /// Transport implementation
    transport: T,
    /// Connected peers mapped by their node IDs
    peers: Arc<RwLock<HashMap<NodeId, Arc<RwLock<Peer<T::Stream>>>>>>,
    /// Channel for sending peer events
    event_tx: broadcast::Sender<PeerEvent>,
}

impl<T> Network<T> 
where 
    T: Transport,
    T::Stream: AsyncRead + AsyncWrite + Unpin + Send + Sync + 'static,
{
    /// Creates a new network instance with the specified configuration.
    pub fn new(config: NetworkConfig, transport: T) -> Self {
        let (event_tx, _) = broadcast::channel(100);
        Self {
            config,
            transport,
            peers: Arc::new(RwLock::new(HashMap::new())),
            event_tx,
        }
    }

    /// Starts the network, listening for incoming connections and managing peers.
    ///
    /// This method runs indefinitely until an error occurs or it is explicitly stopped.
    pub async fn start(&mut self) -> Result<()> {
        // Bind to local address
        let listener = TcpListener::bind(self.config.local_addr).await.map_err(|e| {
            Error::network(format!("failed to bind to {}: {}", self.config.local_addr, e))
        })?;

        // Get actual bound address
        let local_addr = listener.local_addr().map_err(|e| {
            Error::network(format!("failed to get local address: {}", e))
        })?;
        println!("Network listening on {}", local_addr);

        // Start background tasks
        let peers = self.peers.clone();
        let config = self.config.clone();
        tokio::spawn(async move {
            Self::cleanup_task(peers, config.keep_alive_interval).await;
        });

        // Accept incoming connections
        loop {
            match listener.accept().await {
                Ok((stream, addr)) => {
                    if self.peers.read().await.len() >= self.config.max_connections {
                        println!("Max connections reached, rejecting {}", addr);
                        continue;
                    }

                    let event_tx = self.event_tx.clone();
                    tokio::spawn(async move {
                        if let Err(e) = Self::handle_connection(stream, addr, event_tx).await {
                            println!("Connection error: {}", e);
                        }
                    });
                }
                Err(e) => {
                    println!("Accept error: {}", e);
                }
            }
        }
    }

    /// Establishes a connection to a remote peer at the specified address.
    ///
    /// Returns an error if the connection cannot be established within the configured timeout.
    pub async fn connect(&self, addr: SocketAddr) -> Result<()> {
        // Connect with timeout
        let stream = tokio::time::timeout(
            self.config.connection_timeout,
            self.transport.connect(addr)
        )
        .await
        .map_err(|_| {
            Error::network(format!("connection timeout to {}", addr))
        })??;

        // Create peer
        let node_id = NodeId::new(&format!("peer-{}", addr));
        let peer = Peer::new(node_id.clone(), addr, stream, self.event_tx.clone());
        let peer = Arc::new(RwLock::new(peer));

        // Add to peers map
        self.peers.write().await.insert(node_id.clone(), peer.clone());

        // Start peer in background
        let peer_id = node_id.clone();
        tokio::spawn(async move {
            let mut peer = peer.write().await;
            if let Err(e) = peer.start().await {
                println!("Peer error for {}: {}", peer_id, e);
            }
        });

        Ok(())
    }

    /// Broadcasts a message to all connected peers.
    ///
    /// Returns an error if the broadcast fails. Individual peer errors are logged but don't cause
    /// the entire broadcast to fail.
    pub async fn broadcast(&self, message: Message) -> Result<()> {
        let peers = self.peers.read().await;
        for peer in peers.values() {
            let mut peer = peer.write().await;
            if let Err(e) = peer.send_message(message.clone()).await {
                println!("Broadcast error to {}: {}", peer.id(), e);
            }
        }
        Ok(())
    }

    /// Handles an incoming connection from a remote peer.
    async fn handle_connection(
        stream: TcpStream,
        addr: SocketAddr,
        event_tx: broadcast::Sender<PeerEvent>,
    ) -> Result<()> {
        println!("New connection from {}", addr);

        // Create peer
        let node_id = NodeId::new(&format!("peer-{}", addr));
        let mut peer = Peer::new(node_id, addr, stream, event_tx);

        // Start peer
        peer.start().await?;

        Ok(())
    }

    /// Periodically removes disconnected peers from the peer list.
    async fn cleanup_task(
        peers: Arc<RwLock<HashMap<NodeId, Arc<RwLock<Peer<T::Stream>>>>>>,
        interval: Duration,
    ) {
        let mut ticker = time::interval(interval);
        loop {
            ticker.tick().await;

            let mut peers = peers.write().await;
            peers.retain(|_, peer| {
                if let Ok(peer) = peer.try_read() {
                    peer.is_alive(interval)
                } else {
                    // If we can't get a read lock, assume the peer is dead
                    false
                }
            });
        }
    }

    /// Returns a mutable reference to the network event receiver.
    ///
    /// Events include peer connections, disconnections, and received messages.
    pub fn events(&self) -> broadcast::Receiver<PeerEvent> {
        self.event_tx.subscribe()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::time::sleep;

    #[tokio::test]
    async fn test_network() {
        // Create network
        let mut config = NetworkConfig::default();
        config.local_addr = "127.0.0.1:0".parse().unwrap();
        let mut network = Network::new(config.clone(), TcpTransport);

        // Start network in background
        let network_handle = tokio::spawn(async move {
            network.start().await.unwrap();
        });

        // Wait for network to start
        sleep(Duration::from_millis(100)).await;

        // Create client network
        let mut client_config = NetworkConfig::default();
        client_config.local_addr = "127.0.0.1:0".parse().unwrap();
        let client = Network::new(client_config, TcpTransport);

        // Connect to server
        client.connect(config.local_addr).await.unwrap();

        // Wait for connection
        sleep(Duration::from_millis(100)).await;

        // Cleanup
        network_handle.abort();
    }
} 