package testutil

import (
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNetwork represents a test network
type TestNetwork struct {
	t      *testing.T
	mu     sync.RWMutex
	conns  map[string]map[string]net.Conn
	addrs  map[string]string
	protos map[string]map[string]bool
}

// NewTestNetwork creates a new test network
func NewTestNetwork(t *testing.T) *TestNetwork {
	return &TestNetwork{
		t:      t,
		conns:  make(map[string]map[string]net.Conn),
		addrs:  make(map[string]string),
		protos: make(map[string]map[string]bool),
	}
}

// AddPeer adds a peer to the network
func (n *TestNetwork) AddPeer(id string) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Create listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()

	// Store peer info
	n.addrs[id] = addr
	n.conns[id] = make(map[string]net.Conn)
	n.protos[id] = make(map[string]bool)

	// Start accepting connections
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			n.handleConnection(id, conn)
		}
	}()

	return addr, nil
}

// Connect connects two peers
func (n *TestNetwork) Connect(a, b string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Get peer addresses
	addrA, ok := n.addrs[a]
	if !ok {
		return fmt.Errorf("peer not found: %s", a)
	}
	addrB, ok := n.addrs[b]
	if !ok {
		return fmt.Errorf("peer not found: %s", b)
	}

	// Create connections
	connAB, err := net.Dial("tcp", addrB)
	if err != nil {
		return err
	}
	connBA, err := net.Dial("tcp", addrA)
	if err != nil {
		connAB.Close()
		return err
	}

	// Store connections
	n.conns[a][b] = connAB
	n.conns[b][a] = connBA

	return nil
}

// Disconnect disconnects two peers
func (n *TestNetwork) Disconnect(a, b string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if conn, ok := n.conns[a][b]; ok {
		conn.Close()
		delete(n.conns[a], b)
	}
	if conn, ok := n.conns[b][a]; ok {
		conn.Close()
		delete(n.conns[b], a)
	}
}

// IsConnected checks if two peers are connected
func (n *TestNetwork) IsConnected(a, b string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	_, ok := n.conns[a][b]
	return ok
}

// AddProtocol adds a protocol to a peer
func (n *TestNetwork) AddProtocol(peer string, proto string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.protos[peer][proto] = true
}

// RemoveProtocol removes a protocol from a peer
func (n *TestNetwork) RemoveProtocol(peer string, proto string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	delete(n.protos[peer], proto)
}

// HasProtocol checks if a peer supports a protocol
func (n *TestNetwork) HasProtocol(peer string, proto string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return n.protos[peer][proto]
}

// RequireConnected asserts that two peers are connected
func (n *TestNetwork) RequireConnected(t *testing.T, a, b string) {
	require.True(t, n.IsConnected(a, b), "Expected peers to be connected")
}

// RequireNotConnected asserts that two peers are not connected
func (n *TestNetwork) RequireNotConnected(t *testing.T, a, b string) {
	require.False(t, n.IsConnected(a, b), "Expected peers to not be connected")
}

// RequireProtocol asserts that a peer supports a protocol
func (n *TestNetwork) RequireProtocol(t *testing.T, peer string, proto string) {
	require.True(t, n.HasProtocol(peer, proto), "Expected peer to support protocol")
}

// RequireNoProtocol asserts that a peer does not support a protocol
func (n *TestNetwork) RequireNoProtocol(t *testing.T, peer string, proto string) {
	require.False(t, n.HasProtocol(peer, proto), "Expected peer to not support protocol")
}

// WithTestNetwork runs a test with a network
func WithTestNetwork(t *testing.T, fn func(*TestNetwork)) {
	n := NewTestNetwork(t)
	fn(n)
}

// handleConnection handles an incoming connection
func (n *TestNetwork) handleConnection(id string, conn net.Conn) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Get remote address
	remoteAddr := conn.RemoteAddr().String()

	// Find peer with matching address
	for peerId, addr := range n.addrs {
		if addr == remoteAddr {
			n.conns[id][peerId] = conn
			return
		}
	}

	// No matching peer found, close connection
	conn.Close()
}

// TestNetworkBuilder helps build test networks
type TestNetworkBuilder struct {
	t       *testing.T
	network *TestNetwork
	peers   []string
}

// NewTestNetworkBuilder creates a new network builder
func NewTestNetworkBuilder(t *testing.T) *TestNetworkBuilder {
	return &TestNetworkBuilder{
		t:       t,
		network: NewTestNetwork(t),
		peers:   make([]string, 0),
	}
}

// AddPeers adds peers to the network
func (b *TestNetworkBuilder) AddPeers(n int) *TestNetworkBuilder {
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("peer-%d", len(b.peers))
		_, err := b.network.AddPeer(id)
		require.NoError(b.t, err)
		b.peers = append(b.peers, id)
	}
	return b
}

// ConnectAll connects all peers to each other
func (b *TestNetworkBuilder) ConnectAll() *TestNetworkBuilder {
	for i := 0; i < len(b.peers); i++ {
		for j := i + 1; j < len(b.peers); j++ {
			err := b.network.Connect(b.peers[i], b.peers[j])
			require.NoError(b.t, err)
		}
	}
	return b
}

// AddProtocol adds a protocol to all peers
func (b *TestNetworkBuilder) AddProtocol(proto string) *TestNetworkBuilder {
	for _, peer := range b.peers {
		b.network.AddProtocol(peer, proto)
	}
	return b
}

// Build returns the network and peer list
func (b *TestNetworkBuilder) Build() (*TestNetwork, []string) {
	return b.network, b.peers
}
