package substrate

import (
	"testing"
	"time"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetValidatorWeight(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	// Wait for metadata to be loaded
	time.Sleep(2 * time.Second)

	// Create test accounts
	source := createTestAccount(t)
	target := createTestAccount(t)

	// Test getting weight when none is set
	netuid := types.U16(1) // Test on subnet 1
	weight, err := client.GetValidatorWeight(netuid, source, target)
	require.NoError(t, err)
	assert.Equal(t, types.U16(0), weight)

	// TODO: Add test for getting weight after it's set
	// This requires implementing the SetWeight extrinsic
}

func TestGetAllValidatorWeights(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	// Wait for metadata to be loaded
	time.Sleep(2 * time.Second)

	// Create test account
	source := createTestAccount(t)

	// Test getting weights when none are set
	netuid := types.U16(1) // Test on subnet 1
	_, err := client.GetAllValidatorWeights(netuid, source)
	require.Error(t, err) // Should fail since subnet doesn't exist yet
	assert.Contains(t, err.Error(), "subnet 1 not found")

	// TODO: Add test for getting weights after they're set
	// This requires implementing the SetWeight extrinsic and registering a subnet
}

func TestSubscribeValidatorWeights(t *testing.T) {
	client := getTestClient(t)
	defer client.Close()

	// Wait for metadata to be loaded
	time.Sleep(2 * time.Second)

	// Create test account
	source := createTestAccount(t)

	// Subscribe to weight changes
	netuid := types.U16(1) // Test on subnet 1
	ch, err := client.SubscribeValidatorWeights(netuid, source)
	require.NoError(t, err)

	// Start a goroutine to read from the channel
	done := make(chan struct{})
	var receivedWeights []Weight
	go func() {
		defer close(done)
		for weight := range ch {
			receivedWeights = append(receivedWeights, weight)
		}
	}()

	// TODO: Add test for receiving weight updates
	// This requires implementing the SetWeight extrinsic and registering a subnet

	// Wait a bit for any updates
	time.Sleep(time.Second)
	assert.Empty(t, receivedWeights)
}

func createTestAccount(t *testing.T) types.AccountID {
	accountBytes := [32]byte{}
	for i := range accountBytes {
		accountBytes[i] = byte(i + 1)
	}
	var account types.AccountID
	copy(account[:], accountBytes[:])
	return account
}

func getTestClient(t *testing.T) *Client {
	endpoint := getTestEndpoint()
	client, err := NewClient(Config{
		Endpoint: endpoint,
	})
	require.NoError(t, err)
	return client
}
