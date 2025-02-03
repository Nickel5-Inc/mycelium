package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// ConnectionType represents the type of network connection
type ConnectionType int

const (
	// ValidatorConnection represents a persistent connection between validators
	ValidatorConnection ConnectionType = iota
	// MinerConnection represents an ephemeral connection to miners
	MinerConnection
)

// ConnectionConfig holds the configuration for network connections
type ConnectionConfig struct {
	// Type of connection (validator or miner)
	Type ConnectionType
	// Address to connect to
	Address string
	// TLS credentials
	Credentials credentials.TransportCredentials
	// Keepalive interval for persistent connections
	KeepaliveInterval time.Duration
	// Reconnect backoff for failed connections
	ReconnectBackoff time.Duration
	// Maximum number of retry attempts
	MaxRetries int
}

// ConnectionManager handles network connections between nodes
type ConnectionManager struct {
	mu sync.RWMutex
	// Map of validator connections by node ID
	validatorConns map[string]*grpc.ClientConn
	// Configuration for connections
	config ConnectionConfig
	// Channel for connection state changes
	stateChanges chan ConnectionState
}

// ConnectionState represents the current state of a connection
type ConnectionState struct {
	NodeID string
	Type   ConnectionType
	Status string
	Error  error
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config ConnectionConfig) *ConnectionManager {
	return &ConnectionManager{
		validatorConns: make(map[string]*grpc.ClientConn),
		config:         config,
		stateChanges:   make(chan ConnectionState, 100),
	}
}

// ConnectToValidator establishes a persistent connection to another validator
func (cm *ConnectionManager) ConnectToValidator(ctx context.Context, nodeID, address string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if connection already exists
	if _, exists := cm.validatorConns[nodeID]; exists {
		return fmt.Errorf("connection to validator %s already exists", nodeID)
	}

	// Setup gRPC connection with keepalive
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(cm.config.Credentials),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cm.config.KeepaliveInterval,
			Timeout:             cm.config.KeepaliveInterval / 2,
			PermitWithoutStream: true,
		}),
	}

	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to validator %s: %w", nodeID, err)
	}

	cm.validatorConns[nodeID] = conn
	cm.stateChanges <- ConnectionState{
		NodeID: nodeID,
		Type:   ValidatorConnection,
		Status: "connected",
	}

	// Monitor connection state in background
	go cm.monitorConnection(nodeID, conn)

	return nil
}

// monitorConnection watches the connection state and handles reconnection
func (cm *ConnectionManager) monitorConnection(nodeID string, conn *grpc.ClientConn) {
	for {
		state := conn.GetState()
		if state == connectivity.TransientFailure || state == connectivity.Shutdown {
			cm.stateChanges <- ConnectionState{
				NodeID: nodeID,
				Type:   ValidatorConnection,
				Status: "disconnected",
				Error:  fmt.Errorf("connection state: %s", state),
			}

			// Attempt reconnection with backoff
			backoff := cm.config.ReconnectBackoff
			for retries := 0; retries < cm.config.MaxRetries; retries++ {
				if conn.WaitForStateChange(context.Background(), connectivity.TransientFailure) {
					if conn.GetState() == connectivity.Ready {
						cm.stateChanges <- ConnectionState{
							NodeID: nodeID,
							Type:   ValidatorConnection,
							Status: "reconnected",
						}
						break
					}
				}
				time.Sleep(backoff)
				backoff *= 2 // Exponential backoff
			}
		}
		time.Sleep(time.Second)
	}
}

// BroadcastToMiners sends updates to all connected miners
func (cm *ConnectionManager) BroadcastToMiners(ctx context.Context, update []byte) error {
	// Implementation will depend on chosen pub/sub system
	// For now, this is a placeholder for the NATS implementation
	return nil
}

// DisconnectValidator closes a validator connection
func (cm *ConnectionManager) DisconnectValidator(nodeID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.validatorConns[nodeID]
	if !exists {
		return fmt.Errorf("no connection exists for validator %s", nodeID)
	}

	if err := conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection to validator %s: %w", nodeID, err)
	}

	delete(cm.validatorConns, nodeID)
	cm.stateChanges <- ConnectionState{
		NodeID: nodeID,
		Type:   ValidatorConnection,
		Status: "disconnected",
	}

	return nil
}

// GetValidatorConnections returns all active validator connections
func (cm *ConnectionManager) GetValidatorConnections() map[string]*grpc.ClientConn {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Return a copy to prevent external modification
	conns := make(map[string]*grpc.ClientConn, len(cm.validatorConns))
	for k, v := range cm.validatorConns {
		conns[k] = v
	}
	return conns
}

// GetConnectionState returns the channel for monitoring connection state changes
func (cm *ConnectionManager) GetConnectionState() <-chan ConnectionState {
	return cm.stateChanges
}
