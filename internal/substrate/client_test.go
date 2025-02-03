package substrate

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// getTestEndpoint returns the substrate endpoint to use for testing
func getTestEndpoint() string {
	// Check for environment variable first
	if endpoint := os.Getenv("SUBSTRATE_TEST_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	// Default to Bittensor's finney endpoint
	return "wss://entrypoint-finney.opentensor.ai:443"
}

func TestNewClient(t *testing.T) {
	endpoint := getTestEndpoint()
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				Endpoint:   endpoint,
				SS58Format: 42,
			},
			expectError: false,
		},
		{
			name: "invalid endpoint",
			config: Config{
				Endpoint:   "invalid://endpoint",
				SS58Format: 42,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if client == nil {
				t.Error("expected client, got nil")
				return
			}
			defer client.Close()

			// Verify metadata was fetched
			meta := client.GetMetadata()
			if meta == nil {
				t.Error("expected metadata, got nil")
			}
		})
	}
}

func TestQueryStorage(t *testing.T) {
	client, err := NewClient(Config{
		Endpoint:   getTestEndpoint(),
		SS58Format: 42,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query system.number (block number) as a simple test
	data, err := client.QueryStorage(ctx, "System", "Number", nil)
	if err != nil {
		t.Errorf("failed to query storage: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected data, got empty response")
	}
}

func TestSubscribeStorage(t *testing.T) {
	client, err := NewClient(Config{
		Endpoint:   getTestEndpoint(),
		SS58Format: 42,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Subscribe to System.Events
	meta := client.GetMetadata()
	key, err := types.CreateStorageKey(meta, "System", "Events", nil)
	if err != nil {
		t.Fatalf("failed to create storage key: %v", err)
	}

	ch, err := client.SubscribeStorage(ctx, [][]byte{key})
	if err != nil {
		t.Fatalf("failed to subscribe to storage: %v", err)
	}

	// Wait for at least one update or timeout
	select {
	case update := <-ch:
		if len(update.Changes) == 0 {
			t.Error("expected changes, got empty update")
		}
	case <-ctx.Done():
		t.Error("timeout waiting for storage update")
	}
}
