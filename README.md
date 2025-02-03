# Mycelium (Under development, not ready for production use!!!)

A decentralized validator network with secure peer-to-peer communication and stake-based validation.

## Features

- Secure WebSocket-based P2P communication
- Stake-based validator verification
- Dynamic port management with rotation for enhanced security
- Full-mesh network topology
- Database synchronization between nodes
- Configurable through file and environment variables

## Configuration

Configuration can be provided through a `config.json` file and/or environment variables. Environment variables take precedence over file configuration.

### Basic Configuration

```json
{
  "ListenAddr": ":8080",
  "IP": "0.0.0.0",
  "Port": 9090,
  "PortRange": [9090, 9100],
  "NetUID": 1,
  "UID": 0,
  "Hotkey": "5...",  // Your SS58 hotkey address
  "Coldkey": "5...", // Your SS58 coldkey address
  "MinStake": 1000.0,
  "VerifyURL": "http://localhost:5000/verify",
  "PortRotation": {
    "Enabled": true,
    "Interval": "15m",
    "JitterMs": 5000,
    "BatchSize": 5
  },
  "Database": {
    "Host": "localhost",
    "Port": 5432,
    "User": "postgres",
    "Password": "postgres",
    "Database": "mycelium",
    "MaxConns": 10,
    "MaxIdleTime": "3m",
    "HealthCheck": "5s",
    "SSLMode": "disable"
  },
  "Metadata": {
    "Name": "MyValidator",
    "Region": "US-West",
    "Version": "1.0.0"
  },
  "Blacklist": {
    "MaxFailedAttempts": 5,
    "GreylistDuration": "1h",
    "BlacklistThreshold": 10,
    "MinStakeToUnblock": 1000.0
  }
}
```

### Environment Variables

All configuration options can be set via environment variables:

```bash
# Network Settings
MYCELIUM_IP="0.0.0.0"
MYCELIUM_PORT="9090"
MYCELIUM_NET_UID="1"
MYCELIUM_UID="0"

# Validator Identity
MYCELIUM_HOTKEY="5..."
MYCELIUM_COLDKEY="5..."
MYCELIUM_MIN_STAKE="1000.0"

# Port Rotation Settings
MYCELIUM_PORT_ROTATION_ENABLED="true"
MYCELIUM_PORT_ROTATION_INTERVAL="15m"
MYCELIUM_PORT_ROTATION_JITTER="5000"
MYCELIUM_PORT_ROTATION_BATCH_SIZE="5"

# Database Configuration
MYCELIUM_DB_HOST="localhost"
MYCELIUM_DB_PORT="5432"
MYCELIUM_DB_USER="postgres"
MYCELIUM_DB_PASSWORD="postgres"
MYCELIUM_DB_NAME="mycelium"
MYCELIUM_DB_SSL_MODE="disable"

# Verification
MYCELIUM_VERIFY_URL="http://localhost:5000/verify"

# Metadata (prefix with MYCELIUM_METADATA_)
MYCELIUM_METADATA_NAME="MyValidator"
MYCELIUM_METADATA_REGION="US-West"

# Blacklist Configuration
MYCELIUM_BLACKLIST_MAX_FAILED=5
MYCELIUM_BLACKLIST_GREYLIST_DURATION="1h"
MYCELIUM_BLACKLIST_THRESHOLD=10
MYCELIUM_BLACKLIST_MIN_STAKE=1000.0
```

## Port Management

The system implements dynamic port management with several security features:

### Port Range
- Configurable range for WebSocket connections (default: 9090-9100)
- Each outbound connection uses a unique port from this range
- Automatic port availability checking before use

### Port Rotation
- Optional periodic rotation of connection ports
- Configurable rotation interval (default: 15 minutes)
- Batch-based rotation to prevent network disruption
- Random jitter to prevent thundering herd problems
- Graceful connection migration

### Port Rotation Configuration
- `Enabled`: Enable/disable port rotation
- `Interval`: How often to rotate ports
- `JitterMs`: Random delay range for reconnections
- `BatchSize`: Number of connections to rotate at once

## Network Architecture

### Mesh Network
- Full-mesh connectivity between validators
- Automatic connection management
- Periodic mesh connectivity checks
- Gossip-based peer discovery

### Validator Requirements
- Minimum stake requirement (default: 1000 TAO)
- Valid SS58 hotkey and coldkey addresses
- Active status verification
- Automatic cleanup of inactive validators

### Security Features
- Stake-based validation
- Port rotation for connection security
- Configurable connection parameters
- Graceful connection handling
- Mesh-synchronized IP blacklisting

### IP Blacklisting

The system implements a distributed IP blacklisting mechanism to protect against malicious connection attempts:

#### Greylist
- IPs are added to greylist after 5 failed validation attempts
- Greylist entries expire after 1 hour
- Greylisted IPs are temporarily blocked from making new connections

#### Blacklist
- IPs are permanently blacklisted after 10 greylist violations
- Blacklist is synchronized across all validator nodes
- IPs can only be removed from blacklist by acquiring sufficient stake (default: 1000 TAO)
- Blacklist state persists across node restarts

#### Configuration
```json
{
  "Blacklist": {
    "MaxFailedAttempts": 5,
    "GreylistDuration": "1h",
    "BlacklistThreshold": 10,
    "MinStakeToUnblock": 1000.0
  }
}
```

Environment variables:
```bash
MYCELIUM_BLACKLIST_MAX_FAILED=5
MYCELIUM_BLACKLIST_GREYLIST_DURATION="1h"
MYCELIUM_BLACKLIST_THRESHOLD=10
MYCELIUM_BLACKLIST_MIN_STAKE=1000.0
```

#### Mesh Synchronization
- Blacklist updates are propagated through the validator mesh network
- Updates are merged using a "most severe" policy
- Periodic cleanup of expired entries
- Automatic stake checking for blacklisted IPs

## Database Synchronization

The system includes automatic database synchronization between nodes:

- Configurable PostgreSQL connection
- Health checking
- Connection pooling
- Automatic sync management

## Development

### Prerequisites
- Go 1.20 or higher
- PostgreSQL 12 or higher
- Access to a validator verification endpoint

### Building
```bash
go build ./cmd/mycelium
```

### Running
```bash
./mycelium
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Security

For security concerns, please email security@yourdomain.com or open a security advisory. 
