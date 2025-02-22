# Mycelium (Under development, not ready for production use!!!)

A decentralized validator network with secure peer-to-peer communication, distributed transactions, and configurable sharding.

## Features

### Network Layer
- Unified P2P communication with connection management
- TLS encryption with certificate management
- Connection retry with exponential backoff
- Heartbeat monitoring and health checks
- Comprehensive metrics collection
- Role-based node types (validator/miner)

### Database Layer
- Multi-backend support (SQLite and PostgreSQL)
  - Advanced connection pooling
  - Statement caching
  - Type-safe query building
  - SQL parsing and analysis
  - Schema versioning and migrations
  - Role-based access control
  - Performance metrics

### Distributed Transactions
- Two-phase commit protocol (2PC)
- Transaction coordinator
- Participant management
- Automatic timeout handling
- Transaction recovery
- Event notifications
- Comprehensive metrics

### Replication System
- WAL-based replication with version vectors
- SQL-aware conflict detection
- Transaction-level conflict detection
- Primary/Secondary node roles
- Automatic failover and recovery
- Connection retry with backoff
- Heartbeat monitoring
- Comprehensive metrics

### Sharding System
- Configurable table policies
- Flexible replication factors
- Validator/miner node support
- Range-based sharding
- Custom shard functions
- Automatic rebalancing
- Shard-aware routing

### Monitoring
- Detailed replication metrics
- Connection and transport metrics
- Transaction metrics
- Node health monitoring
- Performance tracking
- Shard distribution stats

## Configuration

Configuration is handled through environment variables with the prefix `MYCELIUM_`. Key configuration options include:

### Network
- `HOST` - Host address (default: "0.0.0.0")
- `PORT` - Main service port (default: 8080)
- `P2P_PORT` - P2P communication port (default: 9090)
- `NETWORK_ID` - Network identifier (default: "testnet")
- `PROTOCOL_VERSION` - Protocol version (default: "1.0.0")
- `MIN_PROTOCOL_VERSION` - Minimum compatible protocol version (default: "1.0.0")

### Node Identity
- `NODE_TYPE` - Type of node ("validator" or "miner", required)
- `PUBLIC_KEY` - Node's public key (required)
- `PRIVATE_KEY` - Node's private key (required)
- `VALIDATOR_WEIGHT` - Node's validator weight (default: 1)
- `MINER_CAPACITY` - Node's storage capacity in GB (required for miners)

### Database
- `DATABASE_TYPE` - Database backend type ("sqlite" or "postgres", default: "sqlite")
- `DATABASE_URL` - Database connection URL (required for PostgreSQL)
- `DATABASE_PATH` - SQLite database file path (default: "mycelium.db")
- `DATABASE_MAX_CONNECTIONS` - Maximum number of database connections (default: 10)
- `DATABASE_IDLE_TIMEOUT` - Connection idle timeout in seconds (default: 300)
- `DATABASE_CONNECT_TIMEOUT` - Connection timeout in seconds (default: 10)
- `DATABASE_SSL_MODE` - PostgreSQL SSL mode (default: "prefer")
- `DATABASE_STATEMENT_CACHE` - Statement cache size (default: 1000)

### Transactions
- `TRANSACTION_TIMEOUT` - Default transaction timeout in seconds (default: 30)
- `PREPARE_TIMEOUT` - 2PC prepare phase timeout in seconds (default: 5)
- `COMMIT_TIMEOUT` - 2PC commit phase timeout in seconds (default: 5)
- `MAX_TRANSACTIONS` - Maximum concurrent transactions (default: 1000)
- `CLEANUP_INTERVAL` - Transaction cleanup interval in seconds (default: 60)

### Replication
- `REPLICATION_MODE` - Replication mode ("primary" or "secondary", default: "secondary")
- `REPLICATION_PEERS` - Comma-separated list of peer addresses
- `REPLICATION_HEARTBEAT_INTERVAL` - Heartbeat interval in seconds (default: 5)
- `REPLICATION_HEARTBEAT_TIMEOUT` - Heartbeat timeout in seconds (default: 15)
- `REPLICATION_RETRY_INITIAL` - Initial retry delay in seconds (default: 1)
- `REPLICATION_RETRY_MAX` - Maximum retry delay in seconds (default: 60)
- `REPLICATION_RETRY_MULTIPLIER` - Retry backoff multiplier (default: 2.0)

### Sharding
- `DEFAULT_REPLICATION_FACTOR` - Default table replication factor (default: 0.5)
- `MIN_VALIDATOR_COPIES` - Minimum validator copies per shard (default: 2)
- `MAX_SHARD_SIZE` - Maximum shard size in bytes (default: 1GB)
- `REBALANCE_INTERVAL` - Shard rebalancing interval in seconds (default: 3600)
- `LOAD_THRESHOLD` - Node load threshold for rebalancing (default: 0.8)

### Security
- `TLS_CERT_PATH` - Path to TLS certificate file
- `TLS_KEY_PATH` - Path to TLS private key file
- `TLS_CA_PATH` - Path to CA certificate file
- `TLS_VERIFY_CLIENT` - Whether to verify client certificates (default: true)
- `TLS_VERIFY_HOSTNAME` - Whether to verify hostnames (default: true)

### Performance
- `MAX_PEERS` - Maximum number of peers (default: 50)
- `CONNECTION_LIMIT` - Maximum concurrent connections (default: 100)
- `CONNECTION_TIMEOUT` - Connection timeout (default: 30s)
- `PING_INTERVAL` - Peer ping interval (default: 30s)
- `SYNC_INTERVAL` - State sync interval (default: 60s)

## Development

### Prerequisites
- Rust 1.70 or later
- PostgreSQL 13 or later (optional)
- SQLite 3.35 or later

### Building
```bash
cargo build --release
```

### Running
```bash
cargo run --release
```

### Testing
```bash
cargo test
```

## Usage Examples

### Basic Setup
```rust
use mycelium::{Network, NetworkConfig, Database, DatabaseConfig};

// Create network
let network = Network::new(NetworkConfig::default());

// Create database
let db = Database::connect(DatabaseConfig::default()).await?;

// Start services
network.start().await?;
```

### Distributed Transactions
```rust
use mycelium::transaction::{TransactionManager, TransactionConfig};

// Create transaction manager
let tx_manager = TransactionManager::new(config, transport, shard_manager);

// Start transaction
let tx_id = tx_manager.begin().await?;

// Add operations
tx_manager.add_operation(
    tx_id,
    "UPDATE users SET name = ? WHERE id = ?",
    vec![name.into(), id.into()],
).await?;

// Commit (starts 2PC)
tx_manager.commit(tx_id).await?;

// Subscribe to events
let mut events = tx_manager.subscribe();
while let Ok(event) = events.recv().await {
    match event {
        TransactionEvent::StateChanged { tx_id, old_state, new_state } => {
            println!("Transaction {} state: {:?} -> {:?}", tx_id, old_state, new_state);
        }
        TransactionEvent::Completed { tx_id, success, duration } => {
            println!("Transaction {} completed: success={}, duration={}ms", 
                tx_id, success, duration);
        }
    }
}
```

### Sharding Configuration
```rust
use mycelium::shard::{ShardManager, TablePolicy, ShardConfig};

// Create shard manager
let shard_manager = ShardManager::new(ShardConfig::default());

// Define table policy
let policy = TablePolicy {
    table: "users".to_string(),
    replication_factor: 0.5,  // 50% of nodes
    validator_only: true,
    min_validator_copies: 2,
    shard_keys: vec!["user_id".to_string()],
    range_sharding: true,
};

// Apply policy
shard_manager.set_policy(policy).await?;

// Get shard for query
let shard = shard_manager.get_shard(
    "users",
    "SELECT * FROM users WHERE user_id = 1"
).await?;
```

## Metrics

The system exposes detailed metrics including:
- Transaction metrics (commits, rollbacks, timeouts)
- Replication metrics (WAL entries, bytes, lag)
- Connection metrics (retries, failures, active)
- Node health metrics (status, role, uptime)
- Performance metrics (latency, throughput)
- Shard metrics (distribution, balance, load)

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Security

For security concerns, please email security@yourdomain.com or open a security advisory. 
