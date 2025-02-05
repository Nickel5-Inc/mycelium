package substrate_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"mycelium/internal/substrate"

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
	wallet, err := substrate.NewWallet("5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY", "")
	require.NoError(t, err)
	require.NotNil(t, wallet)

	expectedPubKey, err := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, expectedPubKey, wallet.Hotkey.PublicKey)
}
