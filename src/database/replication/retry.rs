use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::{broadcast, RwLock};
use tokio::time::sleep;
use serde::{Serialize, Deserialize};

use crate::database::{Error, Result};
use crate::network::Network;

/// Retry configuration
#[derive(Debug, Clone)]
pub struct RetryConfig {
    /// Initial retry delay
    pub initial_delay: Duration,
    /// Maximum retry delay
    pub max_delay: Duration,
    /// Backoff multiplier
    pub backoff_multiplier: f64,
    /// Maximum retry attempts (None for infinite)
    pub max_attempts: Option<u32>,
    /// Reset retry count after successful connection
    pub reset_on_success: bool,
}

impl Default for RetryConfig {
    fn default() -> Self {
        Self {
            initial_delay: Duration::from_secs(1),
            max_delay: Duration::from_secs(60),
            backoff_multiplier: 2.0,
            max_attempts: None,
            reset_on_success: true,
        }
    }
}

/// Retry state for a peer
#[derive(Debug, Clone)]
struct RetryState {
    /// Peer address
    addr: SocketAddr,
    /// Current retry attempt
    attempts: u32,
    /// Next retry time
    next_retry: Instant,
    /// Current delay
    current_delay: Duration,
}

/// Retry events
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum RetryEvent {
    /// Connection retry attempt
    Retry {
        addr: SocketAddr,
        attempt: u32,
        next_delay: Duration,
    },
    /// Maximum retry attempts reached
    MaxAttemptsReached {
        addr: SocketAddr,
        attempts: u32,
    },
    /// Connection succeeded
    Connected {
        addr: SocketAddr,
        attempts: u32,
    },
}

/// Connection retry manager
#[derive(Debug)]
pub struct RetryManager {
    /// Configuration
    config: RetryConfig,
    /// Network reference
    network: Arc<Network>,
    /// Retry states
    states: Arc<RwLock<HashMap<SocketAddr, RetryState>>>,
    /// Event sender
    event_tx: broadcast::Sender<RetryEvent>,
}

impl RetryManager {
    /// Create a new retry manager
    pub fn new(network: Arc<Network>, config: RetryConfig) -> Self {
        let (event_tx, _) = broadcast::channel(100);
        Self {
            config,
            network,
            states: Arc::new(RwLock::new(HashMap::new())),
            event_tx,
        }
    }

    /// Start retrying connections to a peer
    pub async fn start_retrying(&self, addr: SocketAddr) -> Result<()> {
        let mut states = self.states.write().await;
        if !states.contains_key(&addr) {
            states.insert(addr, RetryState {
                addr,
                attempts: 0,
                next_retry: Instant::now(),
                current_delay: self.config.initial_delay,
            });
            
            // Start retry task
            let config = self.config.clone();
            let network = self.network.clone();
            let states = self.states.clone();
            let event_tx = self.event_tx.clone();
            
            tokio::spawn(async move {
                Self::retry_task(addr, config, network, states, event_tx).await;
            });
        }
        Ok(())
    }

    /// Stop retrying connections to a peer
    pub async fn stop_retrying(&self, addr: SocketAddr) {
        self.states.write().await.remove(&addr);
    }

    /// Handle successful connection
    pub async fn handle_connected(&self, addr: SocketAddr) {
        let mut states = self.states.write().await;
        if let Some(state) = states.get_mut(&addr) {
            let attempts = state.attempts;
            if self.config.reset_on_success {
                state.attempts = 0;
                state.current_delay = self.config.initial_delay;
            }
            let _ = self.event_tx.send(RetryEvent::Connected {
                addr,
                attempts,
            });
        }
    }

    /// Subscribe to retry events
    pub fn subscribe(&self) -> broadcast::Receiver<RetryEvent> {
        self.event_tx.subscribe()
    }

    /// Retry task for a peer
    async fn retry_task(
        addr: SocketAddr,
        config: RetryConfig,
        network: Arc<Network>,
        states: Arc<RwLock<HashMap<SocketAddr, RetryState>>>,
        event_tx: broadcast::Sender<RetryEvent>,
    ) {
        loop {
            // Check if we should continue retrying
            let should_retry = {
                let states = states.read().await;
                states.contains_key(&addr)
            };
            
            if !should_retry {
                break;
            }

            // Get current state
            let (should_attempt, delay) = {
                let mut states = states.write().await;
                if let Some(state) = states.get_mut(&addr) {
                    let now = Instant::now();
                    if now >= state.next_retry {
                        // Check max attempts
                        if let Some(max) = config.max_attempts {
                            if state.attempts >= max {
                                let _ = event_tx.send(RetryEvent::MaxAttemptsReached {
                                    addr,
                                    attempts: state.attempts,
                                });
                                return;
                            }
                        }

                        // Update state for next attempt
                        state.attempts += 1;
                        state.current_delay = Duration::from_secs_f64(
                            (state.current_delay.as_secs_f64() * config.backoff_multiplier)
                                .min(config.max_delay.as_secs_f64())
                        );
                        state.next_retry = now + state.current_delay;

                        let _ = event_tx.send(RetryEvent::Retry {
                            addr,
                            attempt: state.attempts,
                            next_delay: state.current_delay,
                        });

                        (true, state.current_delay)
                    } else {
                        (false, state.next_retry - now)
                    }
                } else {
                    return;
                }
            };

            // Attempt connection
            if should_attempt {
                if network.connect(addr).await.is_ok() {
                    break;
                }
            }

            // Wait for next attempt
            sleep(delay).await;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::network::{NetworkConfig, TcpTransport};
    use tokio::time::sleep;

    #[tokio::test]
    async fn test_retry_manager() {
        // Create network
        let mut config = NetworkConfig::default();
        config.local_addr = "127.0.0.1:0".parse().unwrap();
        let network = Arc::new(Network::new(config, TcpTransport));

        // Create retry manager
        let retry_config = RetryConfig {
            initial_delay: Duration::from_millis(100),
            max_delay: Duration::from_millis(500),
            backoff_multiplier: 2.0,
            max_attempts: Some(3),
            reset_on_success: true,
        };
        let manager = RetryManager::new(network.clone(), retry_config);

        // Subscribe to events
        let mut events = manager.subscribe();

        // Start retrying an unreachable address
        let addr = "127.0.0.1:1".parse().unwrap();
        manager.start_retrying(addr).await.unwrap();

        // Wait for max attempts
        let mut retry_count = 0;
        let mut max_attempts_reached = false;

        while let Ok(event) = events.recv().await {
            match event {
                RetryEvent::Retry { attempt, .. } => {
                    retry_count = attempt;
                }
                RetryEvent::MaxAttemptsReached { attempts, .. } => {
                    assert_eq!(attempts, 3);
                    max_attempts_reached = true;
                    break;
                }
                _ => {}
            }
        }

        assert_eq!(retry_count, 3);
        assert!(max_attempts_reached);

        // Test successful connection
        let mut server_config = NetworkConfig::default();
        server_config.local_addr = "127.0.0.1:0".parse().unwrap();
        let server = Arc::new(Network::new(server_config.clone(), TcpTransport));

        // Start server
        let server_clone = server.clone();
        tokio::spawn(async move {
            server_clone.start().await.unwrap();
        });

        sleep(Duration::from_millis(100)).await;

        // Start retrying to server
        manager.start_retrying(server_config.local_addr).await.unwrap();

        // Wait for connection
        let mut connected = false;
        while let Ok(event) = events.recv().await {
            if let RetryEvent::Connected { .. } = event {
                connected = true;
                break;
            }
        }

        assert!(connected);
    }
} 