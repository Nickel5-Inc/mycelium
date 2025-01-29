package config

import (
	"encoding/json"
	"fmt"
	"mycelium/internal/database"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Validation constants
const (
	MinPort         = 1024
	MaxPort         = 65535
	DefaultWSPort   = 9090   // Default WebSocket port
	WSPortRangeMin  = 9090   // Start of WebSocket port range
	WSPortRangeMax  = 9100   // End of WebSocket port range (inclusive)
	SS58Prefix      = "5"    // Substrate/Polkadot SS58 prefix
	DefaultMinStake = 1000.0 // Default minimum stake requirement (1k TAO)
)

// BlacklistConfig holds the configuration for IP blacklisting
type BlacklistConfig struct {
	MaxFailedAttempts  int           `json:"max_failed_attempts"`
	GreylistDuration   time.Duration `json:"greylist_duration"`
	BlacklistThreshold int           `json:"blacklist_threshold"`
	MinStakeToUnblock  float64       `json:"min_stake_to_unblock"`
}

type Config struct {
	// Network settings
	ListenAddr string
	IP         string
	Port       uint16
	PortRange  [2]uint16 // [min, max] port range for WebSocket connections

	// Port rotation settings
	PortRotation struct {
		Enabled   bool          // Whether to enable port rotation
		Interval  time.Duration // How often to rotate ports
		JitterMs  int64         // Random jitter in milliseconds to add to rotation
		BatchSize int           // How many connections to rotate at once
	}

	// Validator identity
	NetUID   uint16
	UID      uint16
	Hotkey   string
	Coldkey  string
	Stake    float64
	MinStake float64 // Minimum stake required for validators

	// Database configuration
	Database database.Config

	// Verification endpoint
	VerifyURL string

	// Additional metadata
	Metadata map[string]string

	// Blacklist configuration
	Blacklist BlacklistConfig `json:"blacklist"`
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Network validation
	if c.IP != "" {
		if ip := net.ParseIP(c.IP); ip == nil {
			return fmt.Errorf("invalid IP address: %s", c.IP)
		}
	}

	// Port range validation
	if c.PortRange[0] > c.PortRange[1] {
		return fmt.Errorf("invalid port range: min port cannot be greater than max port")
	}
	if c.PortRange[0] < MinPort || c.PortRange[1] > MaxPort {
		return fmt.Errorf("port range must be between %d and %d", MinPort, MaxPort)
	}
	if c.Port < c.PortRange[0] || c.Port > c.PortRange[1] {
		return fmt.Errorf("listen port %d must be within configured port range [%d, %d]",
			c.Port, c.PortRange[0], c.PortRange[1])
	}

	// Validator identity validation
	if c.Hotkey == "" {
		return fmt.Errorf("hotkey is required")
	}
	if !isValidSS58Address(c.Hotkey) {
		return fmt.Errorf("invalid hotkey format: must be SS58 address")
	}

	if c.Coldkey == "" {
		return fmt.Errorf("coldkey is required")
	}
	if !isValidSS58Address(c.Coldkey) {
		return fmt.Errorf("invalid coldkey format: must be SS58 address")
	}

	if c.MinStake < 0 {
		return fmt.Errorf("minimum stake cannot be negative")
	}

	// Database validation
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("invalid database config: %w", err)
	}

	return nil
}

// isValidSS58Address checks if the given string is a valid SS58 address
func isValidSS58Address(addr string) bool {
	// Basic format check - should start with SS58 prefix and be the correct length
	if !strings.HasPrefix(addr, SS58Prefix) {
		return false
	}
	// TODO: Add more comprehensive SS58 validation
	return len(addr) == 48 // Standard SS58 address length
}

// Load loads configuration from file and environment variables.
// Environment variables take precedence over file configuration.
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr: ":8080",
		IP:         "0.0.0.0",
		Port:       DefaultWSPort,
		PortRange:  [2]uint16{WSPortRangeMin, WSPortRangeMax},
		PortRotation: struct {
			Enabled   bool
			Interval  time.Duration
			JitterMs  int64
			BatchSize int
		}{
			Enabled:   true,
			Interval:  time.Minute * 15,
			JitterMs:  5000,
			BatchSize: 5,
		},
		NetUID:   1,
		UID:      0,
		MinStake: DefaultMinStake,
		Database: database.Config{
			Host:        "localhost",
			Port:        5432,
			User:        "postgres",
			Password:    "postgres",
			Database:    "mycelium",
			MaxConns:    10,
			MaxIdleTime: time.Minute * 3,
			HealthCheck: time.Second * 5,
			SSLMode:     "disable",
		},
		VerifyURL: "http://localhost:5000/verify",
		Metadata:  make(map[string]string),
		Blacklist: BlacklistConfig{
			MaxFailedAttempts:  5,
			GreylistDuration:   time.Hour,
			BlacklistThreshold: 10,
			MinStakeToUnblock:  1000.0,
		},
	}

	// Try loading from config file
	if err := cfg.loadFromFile(); err != nil {
		// Log but don't fail if config file is missing
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error loading config file: %w", err)
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// loadFromFile loads configuration from config.json in the current directory
func (c *Config) loadFromFile() error {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return err
	}

	return json.Unmarshal(data, c)
}

// loadFromEnv loads configuration from environment variables
func (c *Config) loadFromEnv() {
	if ip := os.Getenv("MYCELIUM_IP"); ip != "" {
		c.IP = ip
	}
	if port := os.Getenv("MYCELIUM_PORT"); port != "" {
		if p, err := strconv.ParseUint(port, 10, 16); err == nil {
			c.Port = uint16(p)
		}
	}
	if netUID := os.Getenv("MYCELIUM_NET_UID"); netUID != "" {
		if uid, err := strconv.ParseUint(netUID, 10, 16); err == nil {
			c.NetUID = uint16(uid)
		}
	}
	if uid := os.Getenv("MYCELIUM_UID"); uid != "" {
		if id, err := strconv.ParseUint(uid, 10, 16); err == nil {
			c.UID = uint16(id)
		}
	}
	if hotkey := os.Getenv("MYCELIUM_HOTKEY"); hotkey != "" {
		c.Hotkey = hotkey
	}
	if coldkey := os.Getenv("MYCELIUM_COLDKEY"); coldkey != "" {
		c.Coldkey = coldkey
	}
	if stake := os.Getenv("MYCELIUM_MIN_STAKE"); stake != "" {
		if s, err := strconv.ParseFloat(stake, 64); err == nil {
			c.MinStake = s
		}
	}
	if verifyURL := os.Getenv("MYCELIUM_VERIFY_URL"); verifyURL != "" {
		c.VerifyURL = verifyURL
	}

	// Database configuration
	if dbHost := os.Getenv("MYCELIUM_DB_HOST"); dbHost != "" {
		c.Database.Host = dbHost
	}
	if dbPort := os.Getenv("MYCELIUM_DB_PORT"); dbPort != "" {
		if p, err := strconv.Atoi(dbPort); err == nil {
			c.Database.Port = p
		}
	}
	if dbUser := os.Getenv("MYCELIUM_DB_USER"); dbUser != "" {
		c.Database.User = dbUser
	}
	if dbPass := os.Getenv("MYCELIUM_DB_PASSWORD"); dbPass != "" {
		c.Database.Password = dbPass
	}
	if dbName := os.Getenv("MYCELIUM_DB_NAME"); dbName != "" {
		c.Database.Database = dbName
	}
	if dbSSL := os.Getenv("MYCELIUM_DB_SSL_MODE"); dbSSL != "" {
		c.Database.SSLMode = dbSSL
	}

	// Load additional metadata from MYCELIUM_METADATA_*
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "MYCELIUM_METADATA_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimPrefix(parts[0], "MYCELIUM_METADATA_")
				c.Metadata[key] = parts[1]
			}
		}
	}

	// Port rotation settings
	if enabled := os.Getenv("MYCELIUM_PORT_ROTATION_ENABLED"); enabled != "" {
		c.PortRotation.Enabled = enabled == "true"
	}
	if interval := os.Getenv("MYCELIUM_PORT_ROTATION_INTERVAL"); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil {
			c.PortRotation.Interval = d
		}
	}
	if jitter := os.Getenv("MYCELIUM_PORT_ROTATION_JITTER"); jitter != "" {
		if j, err := strconv.ParseInt(jitter, 10, 64); err == nil {
			c.PortRotation.JitterMs = j
		}
	}
	if batchSize := os.Getenv("MYCELIUM_PORT_ROTATION_BATCH_SIZE"); batchSize != "" {
		if b, err := strconv.Atoi(batchSize); err == nil {
			c.PortRotation.BatchSize = b
		}
	}

	// Blacklist configuration
	if maxFailed := os.Getenv("MYCELIUM_BLACKLIST_MAX_FAILED"); maxFailed != "" {
		if m, err := strconv.Atoi(maxFailed); err == nil {
			c.Blacklist.MaxFailedAttempts = m
		}
	}
	if duration := os.Getenv("MYCELIUM_BLACKLIST_GREYLIST_DURATION"); duration != "" {
		if d, err := time.ParseDuration(duration); err == nil {
			c.Blacklist.GreylistDuration = d
		}
	}
	if threshold := os.Getenv("MYCELIUM_BLACKLIST_THRESHOLD"); threshold != "" {
		if t, err := strconv.Atoi(threshold); err == nil {
			c.Blacklist.BlacklistThreshold = t
		}
	}
	if minStake := os.Getenv("MYCELIUM_BLACKLIST_MIN_STAKE"); minStake != "" {
		if s, err := strconv.ParseFloat(minStake, 64); err == nil {
			c.Blacklist.MinStakeToUnblock = s
		}
	}
}
