mod conflict;
mod sql;
mod transport;
mod heartbeat;
mod retry;
mod metrics;
mod primary;
mod shard;

pub use conflict::{VersionVector, VersionedEntry, ConflictStrategy};
pub use sql::{SqlAnalyzer, TableOperation, OperationType};
pub use transport::{Transport, TransportConfig, Message};
pub use heartbeat::{HeartbeatMonitor, HeartbeatConfig, HeartbeatEvent, NodeStatus, NodeHealth};
pub use retry::{RetryManager, RetryConfig, RetryEvent};
pub use metrics::{MetricsCollector, ReplicationMetrics, NodeMetrics};
pub use primary::{PrimaryManager, PrimaryConfig, PrimaryState, PrimaryInfo, PrimaryEvent};
pub use shard::{ShardManager, ShardConfig, TablePolicy, ShardAssignment, NodeShardInfo};

use std::sync::Arc;
use tokio::sync::{broadcast, RwLock};
use chrono::{DateTime, Utc};
use serde::{Serialize, Deserialize};
use uuid::Uuid;

use super::{Database, Error, Result, Value};

/// Replication node role
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum NodeRole {
    /// Primary node that accepts writes
    Primary,
    /// Secondary node that only accepts reads
    Secondary,
}

// ... rest of existing code ... 