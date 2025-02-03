package substrate

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

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
	origPath := DefaultWalletPath
	DefaultWalletPath = filepath.Join(tmpDir, "wallets")
	defer func() { DefaultWalletPath = origPath }()

	// Test loading wallet
	wallet, err := LoadFromPath(walletName, hotkeyName)
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
			wallet, err := LoadFromPath(tc.walletDir, tc.hotkeyDir)
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

func TestParseKeyFile(t *testing.T) {
	// Test valid key file
	validData := []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n" +
		"0x45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	keypair, err := parseKeyFile(validData)
	require.NoError(t, err)
	require.NotNil(t, keypair)

	expectedPubKey, err := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, expectedPubKey, keypair.PublicKey)
	assert.Equal(t, "0x45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8", keypair.URI)

	// Test invalid key file formats
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "empty file",
			data: []byte{},
		},
		{
			name: "missing private key",
			data: []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		},
		{
			name: "invalid public key hex",
			data: []byte("invalid hex\n0x45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keypair, err := parseKeyFile(tc.data)
			assert.Error(t, err)
			assert.Nil(t, keypair)
		})
	}
}
