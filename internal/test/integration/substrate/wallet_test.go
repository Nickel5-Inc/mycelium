package substrate_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycelium/internal/substrate"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromPath(t *testing.T) {
	// Create temporary directory to simulate ~/.bittensor/wallets
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tmpDir := filepath.Join(home, ".bittensor_test")
	require.NoError(t, os.MkdirAll(tmpDir, 0755))
	defer os.RemoveAll(tmpDir)

	// Create test wallet structure
	walletName := "test_wallet"
	walletDir := filepath.Join(tmpDir, "wallets", walletName)
	require.NoError(t, os.MkdirAll(filepath.Join(walletDir, "hotkeys"), 0755))

	// Create test key files
	hotkeyName := "test_hotkey"
	hotkeyPath := filepath.Join(walletDir, "hotkeys", hotkeyName)
	coldkeyPath := filepath.Join(walletDir, "coldkey")

	hotkeyContent := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n" +
		"0x45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8"
	err = os.WriteFile(hotkeyPath, []byte(hotkeyContent), 0600)
	require.NoError(t, err)

	coldkeyContent := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210\n" +
		"0x45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8"
	err = os.WriteFile(coldkeyPath, []byte(coldkeyContent), 0600)
	require.NoError(t, err)

	// Override default wallet path for testing
	origPath := substrate.DefaultWalletPath
	substrate.DefaultWalletPath = filepath.Join(tmpDir, "wallets")
	defer func() { substrate.DefaultWalletPath = origPath }()

	// Test loading wallet
	wallet, err := substrate.LoadFromPath(walletName, hotkeyName)
	require.NoError(t, err)
	require.NotNil(t, wallet)

	// Verify hotkey public key
	expectedHotkey, err := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, expectedHotkey, wallet.Hotkey.PublicKey)

	// Verify coldkey public key
	expectedColdkey, err := hex.DecodeString("fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
	require.NoError(t, err)
	assert.Equal(t, expectedColdkey, wallet.Coldkey.PublicKey)

	// Test error cases
	testCases := []struct {
		name      string
		walletDir string
		hotkeyDir string
		expectErr bool
	}{
		{
			name:      "non-existent wallet",
			walletDir: "invalid_wallet",
			hotkeyDir: hotkeyName,
			expectErr: true,
		},
		{
			name:      "non-existent hotkey",
			walletDir: walletName,
			hotkeyDir: "invalid_hotkey",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wallet, err := substrate.LoadFromPath(tc.walletDir, tc.hotkeyDir)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, wallet)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, wallet)
			}
		})
	}
}

func TestKeyPair(t *testing.T) {
	tests := []struct {
		name      string
		hotkey    string
		coldkey   string
		expectErr bool
	}{
		{
			name:      "valid keys",
			hotkey:    "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
			coldkey:   "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
			expectErr: false,
		},
		{
			name:      "invalid hotkey format",
			hotkey:    "invalid-key",
			coldkey:   "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
			expectErr: true,
		},
		{
			name:      "invalid coldkey format",
			hotkey:    "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
			coldkey:   "invalid-key",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet, err := substrate.NewWallet(tt.hotkey, tt.coldkey)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, wallet)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, wallet)

			// Test signing and verification
			message := []byte("test message")
			signature, err := wallet.Sign(message)
			require.NoError(t, err)
			assert.True(t, wallet.Verify(message, signature))

			// Test wrong message verification
			wrongMessage := []byte("wrong message")
			assert.False(t, wallet.Verify(wrongMessage, signature))

			// Test address retrieval
			assert.Equal(t, tt.hotkey, wallet.GetAddress())
		})
	}
}

func TestWalletChainInteraction(t *testing.T) {
	// Create a wallet with test keys
	wallet, err := substrate.NewWallet(
		"5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
		"5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
	)
	require.NoError(t, err)
	require.NotNil(t, wallet)

	// Connect to test endpoint
	endpoint := os.Getenv("SUBSTRATE_TEST_ENDPOINT")
	if endpoint == "" {
		endpoint = "wss://entrypoint-finney.opentensor.ai:443"
	}
	client, err := substrate.NewClient(substrate.Config{
		Endpoint:   endpoint,
		SS58Format: 42,
	})
	require.NoError(t, err)
	defer client.Close()

	// Test chain interaction
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query chain state using the wallet's address
	key, err := types.CreateStorageKey(client.GetMetadata(), "System", "Account", []byte(wallet.GetAddress()))
	require.NoError(t, err)
	data, err := client.QueryStorage(ctx, "System", "Account", key)
	require.NoError(t, err)
	assert.NotNil(t, data)

	// Test signature verification on chain
	message := []byte("test message for chain verification")
	signature, err := wallet.Sign(message)
	require.NoError(t, err)

	// Verify the signature using chain's crypto
	valid := wallet.Verify(message, signature)
	assert.True(t, valid)
}
