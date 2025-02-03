package substrate

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
)

// Default Bittensor wallet path
var DefaultWalletPath = "~/.bittensor/wallets"

// Constants for validation
const (
	SS58Prefix = "5" // Substrate/Polkadot SS58 prefix
)

// Keypair represents a cryptographic key pair
type Keypair struct {
	// Public key bytes
	PublicKey []byte
	// Private key URI (seed phrase or raw key)
	URI string
}

// Wallet represents a Bittensor wallet containing a hotkey and coldkey pair
type Wallet struct {
	// Hotkey is used for frequent operations like setting weights
	Hotkey Keypair
	// Coldkey is used for staking operations
	Coldkey Keypair
}

// NewWallet creates a new wallet from SS58 addresses
func NewWallet(hotkey, coldkey string) (*Wallet, error) {
	// Basic validation of SS58 addresses
	if !strings.HasPrefix(hotkey, SS58Prefix) || len(hotkey) != 48 {
		return nil, fmt.Errorf("invalid hotkey address format")
	}
	if !strings.HasPrefix(coldkey, SS58Prefix) || len(coldkey) != 48 {
		return nil, fmt.Errorf("invalid coldkey address format")
	}

	// For now, we'll store the SS58 addresses directly
	// The actual public key bytes will be handled by the Python bridge when needed
	return &Wallet{
		Hotkey: Keypair{
			PublicKey: []byte{}, // Empty for now, will be handled by Python bridge
			URI:       hotkey,
		},
		Coldkey: Keypair{
			PublicKey: []byte{}, // Empty for now, will be handled by Python bridge
			URI:       coldkey,
		},
	}, nil
}

// LoadFromPath loads a wallet from the default Bittensor wallet path
func LoadFromPath(name, hotkey string) (*Wallet, error) {
	// Expand home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	walletPath := strings.Replace(DefaultWalletPath, "~", home, 1)

	// Construct paths
	walletDir := filepath.Join(walletPath, name)
	hotkeyPath := filepath.Join(walletDir, "hotkeys", hotkey)
	coldkeyPath := filepath.Join(walletDir, "coldkey")

	// Read hotkey file
	hotkeyData, err := os.ReadFile(hotkeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading hotkey file: %w", err)
	}

	// Read coldkey file
	coldkeyData, err := os.ReadFile(coldkeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading coldkey file: %w", err)
	}

	// Parse keys
	hotkeypair, err := parseKeyFile(hotkeyData)
	if err != nil {
		return nil, fmt.Errorf("parsing hotkey: %w", err)
	}

	coldkeypair, err := parseKeyFile(coldkeyData)
	if err != nil {
		return nil, fmt.Errorf("parsing coldkey: %w", err)
	}

	return &Wallet{
		Hotkey:  *hotkeypair,
		Coldkey: *coldkeypair,
	}, nil
}

// Sign signs a message using the hotkey
func (w *Wallet) Sign(message []byte) ([]byte, error) {
	sig, err := signature.Sign(message, w.Hotkey.URI)
	if err != nil {
		return nil, fmt.Errorf("signing message: %w", err)
	}
	return sig, nil
}

// Verify verifies a signature using the hotkey's public key
func (w *Wallet) Verify(message, sig []byte) bool {
	pubKeyHex := hex.EncodeToString(w.Hotkey.PublicKey)
	ok, err := signature.Verify(message, sig, pubKeyHex)
	if err != nil {
		return false
	}
	return ok
}

// parseKeyFile parses a key file in the format used by Bittensor
func parseKeyFile(data []byte) (*Keypair, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid key file format")
	}

	// First line should be public key, second line should be private key
	pubKey, err := hex.DecodeString(strings.TrimSpace(lines[0]))
	if err != nil {
		return nil, fmt.Errorf("decoding public key: %w", err)
	}

	// Create keyring pair
	return &Keypair{
		PublicKey: pubKey,
		URI:       strings.TrimSpace(lines[1]),
	}, nil
}
