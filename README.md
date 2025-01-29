# Mycelium

Mycelium is a peer-to-peer network layer for validator nodes, providing secure communication, database synchronization, and validator verification capabilities.

## Features

- 🔐 Validator verification and stake tracking
- 🔄 Peer-to-peer database synchronization
- 🌐 WebSocket-based peer communication
- 💾 PostgreSQL integration with connection pooling
- 🤝 Automatic peer discovery and gossip protocol
- ❤️ Health monitoring and connection management

## Prerequisites

- Go 1.21 or later
- PostgreSQL 12 or later
- Python (for validator verification service)

## Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/mycelium.git
   cd mycelium
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Set up PostgreSQL:
   ```bash
   createdb mycelium
   ```

4. Configure the application:
   ```bash
   # Edit config values in internal/config/config.go
   # or set environment variables (coming soon)
   ```

5. Run the node:
   ```bash
   go run cmd/mycelium/main.go
   ```

## Configuration

Default configuration (can be modified in `internal/config/config.go`):

```go
{
    ListenAddr: ":8080",
    Database: {
        Host:        "localhost",
        Port:        5432,
        User:        "postgres",
        Password:    "postgres",
        Database:    "mycelium",
        MaxConns:    10,
        MaxIdleTime: 3m,
        HealthCheck: 5s,
        SSLMode:     "disable",
    },
    VerifyURL: "http://localhost:5000/verify"
}
```

## Architecture

- `internal/node`: Core node functionality
- `internal/peer`: Peer management and communication
- `internal/database`: Database operations and sync
- `internal/protocol`: Network protocol definitions
- `internal/chain`: Validator chain integration

## Database Synchronization

Mycelium automatically synchronizes database changes between nodes using a change tracking system. Changes are propagated through the peer network using a gossip protocol.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Security

For security concerns, please email security@yourdomain.com or open a security advisory. 