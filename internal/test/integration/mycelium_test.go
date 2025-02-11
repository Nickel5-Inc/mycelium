package integration

import (
	"testing"
	"time"

	"mycelium/internal/config"
	"mycelium/internal/node"
	"mycelium/internal/test/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEndpoint = "wss://entrypoint-finney.opentensor.ai:443"
)

func TestMyceliumNetwork(t *testing.T) {
	// Create test context
	testCtx := testutil.NewTestContext(t)
	ctx := testCtx.Context()

	// Set netUID to 57
	netUID := 57

	// Create test configs for two nodes
	cfg1 := &config.Config{
		ListenAddr:   "127.0.0.1",
		Port:         9944,
		NetUID:       uint16(netUID),
		Hotkey:       "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
		Coldkey:      "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
		Version:      "1.0.0",
		PortRange:    [2]uint16{10000, 10100},
		MinStake:     1000,
		SubstrateURL: "wss://entrypoint-finney.opentensor.ai:443",
		ChainSpec:    "finney",
	}

	cfg2 := &config.Config{
		ListenAddr:   "127.0.0.1",
		Port:         9945,
		NetUID:       uint16(netUID),
		Hotkey:       "5FHneW46xGXgs5mUiveU4sbTyGBzmstUspZC92UhjJM694ty",
		Coldkey:      "5FHneW46xGXgs5mUiveU4sbTyGBzmstUspZC92UhjJM694ty",
		Version:      "1.0.0",
		PortRange:    [2]uint16{10101, 10200},
		MinStake:     1000,
		SubstrateURL: "wss://entrypoint-finney.opentensor.ai:443",
		ChainSpec:    "finney",
	}

	// Create and start nodes
	node1, err := node.NewNode(ctx, cfg1)
	require.NoError(t, err)
	require.NoError(t, node1.Start())
	defer node1.Stop()

	node2, err := node.NewNode(ctx, cfg2)
	require.NoError(t, err)
	require.NoError(t, node2.Start())
	defer node2.Stop()

	// Wait for peer discovery and initial sync
	time.Sleep(5 * time.Second)

	// Verify peer connections
	status1 := node1.GetStatus()
	status2 := node2.GetStatus()
	assert.Equal(t, 1, status1.PeerCount, "Node 1 should have 1 peer")
	assert.Equal(t, 1, status2.PeerCount, "Node 2 should have 1 peer")

	// Test node status
	assert.True(t, status1.IsActive)
	assert.True(t, status2.IsActive)
	assert.Equal(t, "1.0.0", status1.Version)
	assert.Equal(t, "1.0.0", status2.Version)

	// Test metrics
	node1.UpdateMetrics()
	node2.UpdateMetrics()

	status1 = node1.GetStatus()
	status2 = node2.GetStatus()
	assert.NotZero(t, status1.ServingRate)
	assert.NotZero(t, status2.ServingRate)
	assert.NotZero(t, status1.SyncProgress)
	assert.NotZero(t, status2.SyncProgress)
}
