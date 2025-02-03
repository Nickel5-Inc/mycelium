# Mycelium (Under development, not ready for production use!!!)

A decentralized validator network with secure peer-to-peer communication and stake-based validation.

## Features

- WebSocket-based P2P communication with connection management
- Substrate chain integration for network state and validation
- PostgreSQL database for persistent storage
- Prometheus metrics for monitoring
- Environment-based configuration with validation
- Blacklist protection against malicious peers
- Port rotation for improved network resilience

## Configuration

Configuration is handled through environment variables with the prefix `MYCELIUM_`. Key configuration options include:

### Network
- `HOST` - Host address (default: "0.0.0.0")
- `PORT` - Main service port (default: 8080)
- `P2P_PORT` - P2P communication port (default: 9090)
- `WS_PORT` - WebSocket port (default: 8081)
- `NETWORK_ID` - Network identifier (default: "testnet")
- `SUBSTRATE_URL` - Substrate node URL (default: "ws://localhost:9944")
- `VERIFY_URL` - Verification service URL (default: "http://localhost:8081")

### Node Identity
- `HOTKEY` - Node's hot wallet key (required, SS58 format)
- `COLDKEY` - Node's cold wallet key (optional, SS58 format)
- `NODE_TYPE` - Type of node (required)
- `PUBLIC_KEY` - Node's public key (required)
- `PRIVATE_KEY` - Node's private key (required)

### Database
- `DATABASE_URL` - PostgreSQL connection URL (required)

### Security
- `JWT_SECRET` - Secret for JWT token generation (required)
- `MIN_STAKE` - Minimum stake requirement (default: 1000)

### Performance
- `MAX_PEERS` - Maximum number of peers (default: 50)
- `CONNECTION_LIMIT` - Maximum concurrent connections (default: 100)
- `CONNECTION_TIMEOUT` - Connection timeout (default: 30s)
- `DIAL_TIMEOUT` - Dial timeout (default: 5s)
- `PING_INTERVAL` - Peer ping interval (default: 30s)
- `SYNC_INTERVAL` - State sync interval (default: 60s)

### Port Management
- `PORT_RANGE_MIN` - Start of WebSocket port range (default: 9090)
- `PORT_RANGE_MAX` - End of WebSocket port range (default: 9100)
- `PORT_ROTATION_ENABLED` - Enable port rotation (default: true)
- `PORT_ROTATION_INTERVAL` - Port rotation interval
- `PORT_ROTATION_BATCH_SIZE` - Number of ports to rotate at once

### Blacklist Configuration
- `BLACKLIST_MAX_FAILED` - Maximum failed attempts before greylist
- `BLACKLIST_GREYLIST_DURATION` - Duration to keep IPs in greylist
- `BLACKLIST_THRESHOLD` - Threshold for permanent blacklisting
- `BLACKLIST_MIN_STAKE` - Minimum stake to unblock IP

## Architecture

The node consists of several key components:

- `Node`: Core node implementation managing all components
- `WSManager`: WebSocket connection management
- `PeerManager`: Peer discovery and management
- `Metagraph`: Network state management
- `Identity`: Node identity and authentication
- `Database`: Persistent storage interface
- `Metrics`: Prometheus metrics collection

## Development

### Prerequisites
- Go 1.20 or later
- PostgreSQL 13 or later
- Access to a Substrate node

### Building
```bash
go build ./cmd/mycelium
```

### Running
```bash
./mycelium
```

### Testing
```bash
go test ./...
```

## Metrics

The node exposes Prometheus metrics including:
- Peer count
- Active validators
- Serving rate
- Response latency
- Sync progress
- Memory usage
- CPU usage

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Security

For security concerns, please email security@yourdomain.com or open a security advisory. 
