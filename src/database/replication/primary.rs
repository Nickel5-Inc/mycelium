use std::collections::{HashMap, HashSet};
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::{broadcast, RwLock};
use tokio::time::interval;
use serde::{Serialize, Deserialize};
use uuid::Uuid;

use crate::error::{Error, Result};
use super::{NodeRole, NodeStatus};
use super::metrics::MetricsCollector;
use super::transport::{Transport, Message};

/// Primary node state
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum PrimaryState {
    /// Node is a primary
    Active,
    /// Node is a candidate for primary
    Candidate,
    /// Node is a backup primary
    Backup,
    /// Node is not a primary
    Inactive,
}

/// Primary node information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PrimaryInfo {
    /// Node address
    pub addr: SocketAddr,
    /// Node state
    pub state: PrimaryState,
    /// Last heartbeat timestamp
    pub last_heartbeat: i64,
    /// Write sequence number
    pub sequence: u64,
    /// Write regions
    pub regions: HashSet<String>,
    /// Priority (higher is better)
    pub priority: u32,
}

/// Primary election message
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum ElectionMessage {
    /// Vote request
    RequestVote {
        /// Candidate ID
        candidate_id: Uuid,
        /// Candidate address
        candidate_addr: SocketAddr,
        /// Candidate priority
        priority: u32,
        /// Last write sequence
        last_sequence: u64,
    },
    /// Vote response
    Vote {
        /// Candidate ID
        candidate_id: Uuid,
        /// Voter address
        voter_addr: SocketAddr,
        /// Whether the vote was granted
        granted: bool,
    },
    /// Primary announcement
    PrimaryAnnouncement {
        /// Primary address
        addr: SocketAddr,
        /// Write regions
        regions: HashSet<String>,
        /// Term number
        term: u64,
    },
}

/// Primary manager configuration
#[derive(Debug, Clone)]
pub struct PrimaryConfig {
    /// Minimum number of votes needed
    pub min_votes: u32,
    /// Election timeout
    pub election_timeout: Duration,
    /// Primary announcement interval
    pub announcement_interval: Duration,
    /// Node priority
    pub priority: u32,
    /// Write regions
    pub regions: HashSet<String>,
}

impl Default for PrimaryConfig {
    fn default() -> Self {
        Self {
            min_votes: 2,
            election_timeout: Duration::from_secs(5),
            announcement_interval: Duration::from_secs(1),
            priority: 1,
            regions: HashSet::new(),
        }
    }
}

/// Primary node manager
pub struct PrimaryManager {
    /// Configuration
    config: PrimaryConfig,
    /// Transport layer
    transport: Arc<Transport>,
    /// Current state
    state: Arc<RwLock<PrimaryState>>,
    /// Known primaries
    primaries: Arc<RwLock<HashMap<SocketAddr, PrimaryInfo>>>,
    /// Current term
    term: Arc<RwLock<u64>>,
    /// Election state
    election: Arc<RwLock<Option<ElectionState>>>,
    /// Metrics collector
    metrics: Arc<MetricsCollector>,
    /// Event sender
    event_tx: broadcast::Sender<PrimaryEvent>,
}

/// Election state
#[derive(Debug)]
struct ElectionState {
    /// Candidate ID
    candidate_id: Uuid,
    /// Start time
    start_time: Instant,
    /// Received votes
    votes: HashSet<SocketAddr>,
    /// Election term
    term: u64,
}

/// Primary events
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum PrimaryEvent {
    /// State changed
    StateChanged {
        /// Old state
        old_state: PrimaryState,
        /// New state
        new_state: PrimaryState,
    },
    /// Primary changed
    PrimaryChanged {
        /// Old primary
        old_primary: Option<SocketAddr>,
        /// New primary
        new_primary: SocketAddr,
    },
    /// Election started
    ElectionStarted {
        /// Term number
        term: u64,
    },
    /// Election completed
    ElectionCompleted {
        /// Winner address
        winner: SocketAddr,
        /// Term number
        term: u64,
    },
}

impl PrimaryManager {
    /// Create a new primary manager
    pub fn new(config: PrimaryConfig, transport: Arc<Transport>, metrics: Arc<MetricsCollector>) -> Self {
        let (event_tx, _) = broadcast::channel(100);
        Self {
            config,
            transport,
            state: Arc::new(RwLock::new(PrimaryState::Inactive)),
            primaries: Arc::new(RwLock::new(HashMap::new())),
            term: Arc::new(RwLock::new(0)),
            election: Arc::new(RwLock::new(None)),
            metrics,
            event_tx,
        }
    }

    /// Start the primary manager
    pub async fn start(&self) -> Result<()> {
        // Start election timeout
        self.start_election_timeout();
        
        // Start announcement sender
        self.start_announcement_sender();
        
        // Subscribe to transport messages
        let mut rx = self.transport.subscribe();
        let state = self.state.clone();
        let primaries = self.primaries.clone();
        let election = self.election.clone();
        let term = self.term.clone();
        let config = self.config.clone();
        let event_tx = self.event_tx.clone();
        
        tokio::spawn(async move {
            while let Ok((addr, msg)) = rx.recv().await {
                match msg {
                    Message::Custom(data) => {
                        if let Ok(election_msg) = rmp_serde::from_slice::<ElectionMessage>(&data) {
                            match election_msg {
                                ElectionMessage::RequestVote { candidate_id, candidate_addr, priority, last_sequence } => {
                                    let mut current_state = state.write().await;
                                    let current_term = *term.read().await;
                                    
                                    // Vote if we're not a primary and the candidate has higher priority
                                    if *current_state != PrimaryState::Active && priority >= config.priority {
                                        *current_state = PrimaryState::Inactive;
                                        let vote = ElectionMessage::Vote {
                                            candidate_id,
                                            voter_addr: addr,
                                            granted: true,
                                        };
                                        if let Ok(data) = rmp_serde::to_vec(&vote) {
                                            let _ = transport.send_to(candidate_addr, Message::Custom(data)).await;
                                        }
                                    }
                                }
                                ElectionMessage::Vote { candidate_id, voter_addr, granted } => {
                                    let mut election_state = election.write().await;
                                    if let Some(state) = election_state.as_mut() {
                                        if state.candidate_id == candidate_id && granted {
                                            state.votes.insert(voter_addr);
                                            
                                            // Check if we have enough votes
                                            if state.votes.len() >= config.min_votes as usize {
                                                let announcement = ElectionMessage::PrimaryAnnouncement {
                                                    addr,
                                                    regions: config.regions.clone(),
                                                    term: state.term,
                                                };
                                                if let Ok(data) = rmp_serde::to_vec(&announcement) {
                                                    let _ = transport.broadcast(Message::Custom(data)).await;
                                                }
                                                *state.write().await = PrimaryState::Active;
                                                let _ = event_tx.send(PrimaryEvent::StateChanged {
                                                    old_state: PrimaryState::Candidate,
                                                    new_state: PrimaryState::Active,
                                                });
                                            }
                                        }
                                    }
                                }
                                ElectionMessage::PrimaryAnnouncement { addr, regions, term: new_term } => {
                                    let mut primaries = primaries.write().await;
                                    let current_term = *term.read().await;
                                    
                                    if new_term >= current_term {
                                        // Update term
                                        *term.write().await = new_term;
                                        
                                        // Update primary info
                                        primaries.insert(addr, PrimaryInfo {
                                            addr,
                                            state: PrimaryState::Active,
                                            last_heartbeat: chrono::Utc::now().timestamp_millis(),
                                            sequence: 0,
                                            regions,
                                            priority: 0, // Will be updated with next heartbeat
                                        });
                                        
                                        // Update our state if needed
                                        if *state.read().await == PrimaryState::Candidate {
                                            *state.write().await = PrimaryState::Inactive;
                                            let _ = event_tx.send(PrimaryEvent::StateChanged {
                                                old_state: PrimaryState::Candidate,
                                                new_state: PrimaryState::Inactive,
                                            });
                                        }
                                    }
                                }
                            }
                        }
                    }
                    _ => {}
                }
            }
        });

        Ok(())
    }

    /// Start election timeout monitoring
    fn start_election_timeout(&self) {
        let state = self.state.clone();
        let election = self.election.clone();
        let term = self.term.clone();
        let config = self.config.clone();
        let transport = self.transport.clone();
        let event_tx = self.event_tx.clone();

        tokio::spawn(async move {
            let mut interval = interval(config.election_timeout);
            loop {
                interval.tick().await;
                
                // Check if we should start an election
                let current_state = *state.read().await;
                if current_state == PrimaryState::Inactive {
                    // Start election
                    let candidate_id = Uuid::new_v4();
                    let mut election_state = election.write().await;
                    let mut current_term = term.write().await;
                    *current_term += 1;
                    
                    *election_state = Some(ElectionState {
                        candidate_id,
                        start_time: Instant::now(),
                        votes: HashSet::new(),
                        term: *current_term,
                    });
                    
                    // Send vote request
                    let request = ElectionMessage::RequestVote {
                        candidate_id,
                        candidate_addr: transport.local_addr(),
                        priority: config.priority,
                        last_sequence: 0,
                    };
                    
                    if let Ok(data) = rmp_serde::to_vec(&request) {
                        let _ = transport.broadcast(Message::Custom(data)).await;
                    }
                    
                    *state.write().await = PrimaryState::Candidate;
                    let _ = event_tx.send(PrimaryEvent::StateChanged {
                        old_state: PrimaryState::Inactive,
                        new_state: PrimaryState::Candidate,
                    });
                    let _ = event_tx.send(PrimaryEvent::ElectionStarted {
                        term: *current_term,
                    });
                }
            }
        });
    }

    /// Start primary announcement sender
    fn start_announcement_sender(&self) {
        let state = self.state.clone();
        let config = self.config.clone();
        let transport = self.transport.clone();
        let term = self.term.clone();

        tokio::spawn(async move {
            let mut interval = interval(config.announcement_interval);
            loop {
                interval.tick().await;
                
                // Send announcement if we're primary
                if *state.read().await == PrimaryState::Active {
                    let announcement = ElectionMessage::PrimaryAnnouncement {
                        addr: transport.local_addr(),
                        regions: config.regions.clone(),
                        term: *term.read().await,
                    };
                    
                    if let Ok(data) = rmp_serde::to_vec(&announcement) {
                        let _ = transport.broadcast(Message::Custom(data)).await;
                    }
                }
            }
        });
    }

    /// Get current primary state
    pub async fn state(&self) -> PrimaryState {
        *self.state.read().await
    }

    /// Get known primaries
    pub async fn primaries(&self) -> HashMap<SocketAddr, PrimaryInfo> {
        self.primaries.read().await.clone()
    }

    /// Subscribe to primary events
    pub fn subscribe(&self) -> broadcast::Receiver<PrimaryEvent> {
        self.event_tx.subscribe()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;
    use tokio::time::sleep;
    use crate::network::{Network, NetworkConfig, TcpTransport};

    #[tokio::test]
    async fn test_primary_election() {
        // Create networks
        let mut config1 = NetworkConfig::default();
        config1.local_addr = "127.0.0.1:0".parse().unwrap();
        let network1 = Arc::new(Network::new(config1.clone(), TcpTransport));

        let mut config2 = NetworkConfig::default();
        config2.local_addr = "127.0.0.1:0".parse().unwrap();
        let network2 = Arc::new(Network::new(config2.clone(), TcpTransport));

        // Create transports
        let transport1 = Arc::new(Transport::new(network1.clone(), Default::default(), NodeRole::Primary).await.unwrap());
        let transport2 = Arc::new(Transport::new(network2.clone(), Default::default(), NodeRole::Primary).await.unwrap());

        // Create primary managers
        let metrics = Arc::new(MetricsCollector::new());
        let primary_config = PrimaryConfig {
            min_votes: 1,
            election_timeout: Duration::from_millis(100),
            announcement_interval: Duration::from_millis(50),
            priority: 1,
            regions: HashSet::new(),
        };

        let manager1 = PrimaryManager::new(primary_config.clone(), transport1.clone(), metrics.clone());
        let manager2 = PrimaryManager::new(primary_config.clone(), transport2.clone(), metrics.clone());

        // Start managers
        manager1.start().await.unwrap();
        manager2.start().await.unwrap();

        // Connect nodes
        network2.connect(config1.local_addr).await.unwrap();

        // Wait for election
        sleep(Duration::from_millis(200)).await;

        // Verify one node became primary
        let state1 = manager1.state().await;
        let state2 = manager2.state().await;
        assert!(state1 == PrimaryState::Active || state2 == PrimaryState::Active);
        assert!(state1 != state2);
    }
} 