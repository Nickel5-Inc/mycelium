package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfigData represents test configuration data
type TestConfigData struct {
	DataDir     string
	DatabaseURL string
	P2PPort     int
	RPCPort     int
	MetricsPort int
	LogLevel    string
}

// DefaultTestConfigData returns default test configuration
func DefaultTestConfigData(t *testing.T) *TestConfigData {
	return &TestConfigData{
		DataDir:     CreateTestDir(t),
		DatabaseURL: "postgres://postgres:postgres@localhost:5432/mycelium_test",
		P2PPort:     9000,
		RPCPort:     9001,
		MetricsPort: 9002,
		LogLevel:    "debug",
	}
}

// WithTestConfigData runs a test with configuration
func WithTestConfigData(t *testing.T, fn func(*TestConfigData)) {
	cfg := DefaultTestConfigData(t)
	fn(cfg)
}

// CreateTestConfigDataFile creates a test config file
func CreateTestConfigDataFile(t *testing.T, cfg *TestConfigData) string {
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)

	dir := CreateTestDir(t)
	path := filepath.Join(dir, "config.json")
	err = os.WriteFile(path, data, 0644)
	require.NoError(t, err)

	return path
}

// TestDatabaseURL returns test database URL
func TestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://postgres:postgres@localhost:5432/mycelium_test"
}

// TestP2PConfig returns test P2P config
func TestP2PConfig() (int, string) {
	port := 9000
	if p := os.Getenv("TEST_P2P_PORT"); p != "" {
		port = MustParseInt(p)
	}
	return port, "127.0.0.1"
}

// TestRPCConfig returns test RPC config
func TestRPCConfig() (int, string) {
	port := 9001
	if p := os.Getenv("TEST_RPC_PORT"); p != "" {
		port = MustParseInt(p)
	}
	return port, "127.0.0.1"
}

// TestMetricsConfig returns test metrics config
func TestMetricsConfig() (int, string) {
	port := 9002
	if p := os.Getenv("TEST_METRICS_PORT"); p != "" {
		port = MustParseInt(p)
	}
	return port, "127.0.0.1"
}

// TestLogConfig returns test log config
func TestLogConfig() string {
	if level := os.Getenv("TEST_LOG_LEVEL"); level != "" {
		return level
	}
	return "debug"
}

// MustParseInt parses string to int or panics
func MustParseInt(s string) int {
	i, err := parseInt(s)
	if err != nil {
		panic(err)
	}
	return i
}

// parseInt parses string to int
func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}
