package config

import (
	"fmt"
	"mycelium/internal/database"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// DefaultEnvPrefix is the default prefix for environment variables
const (
	DefaultEnvPrefix = "MYCELIUM_"
	MinPort          = 1024
	MaxPort          = 65535
	DefaultWSPort    = 9090   // Default WebSocket port
	WSPortRangeMin   = 9090   // Start of WebSocket port range
	WSPortRangeMax   = 9100   // End of WebSocket port range (inclusive)
	SS58Prefix       = "5"    // Substrate/Polkadot SS58 prefix
	DefaultMinStake  = 1000.0 // Default minimum stake requirement (1k TAO)
)

// Config represents the node configuration
type Config struct {
	// Network configuration
	Host            string
	Port            uint16
	P2PPort         uint16
	WebSocketPort   uint16
	NetworkID       string
	BootstrapNodes  []string
	MaxPeers        int
	ConnectionLimit int
	VerifyURL       string    // URL for verification service
	PortRange       [2]uint16 // [min, max] port range for WebSocket connections
	MinStake        uint64    // Minimum stake requirement
	ListenAddr      string    // Address to listen on (e.g., ":8080")
	SubstrateURL    string    // URL for substrate node
	Version         string    // Node version
	NetUID          uint16    // Network UID
	Coldkey         string    // Cold wallet key

	// Node identity
	Hotkey     string
	NodeType   string
	PublicKey  string
	PrivateKey string

	// Database configuration
	DatabaseURL string

	// Security settings
	JWTSecret string

	// Timeouts and intervals
	ConnectionTimeout time.Duration
	DialTimeout       time.Duration
	PingInterval      time.Duration
	SyncInterval      time.Duration

	// Port rotation settings
	PortRotation struct {
		Enabled   bool
		Interval  time.Duration
		BatchSize int
	}

	// Validator identity
	UID   uint16
	Stake float64

	// Additional metadata
	Metadata map[string]string

	// Blacklist configuration
	Blacklist BlacklistConfig
}

// DatabaseConfig represents database connection configuration
type DatabaseConfig struct {
	Host        string
	Port        uint16
	User        string
	Password    string
	Database    string
	MaxConns    int
	MaxIdleTime time.Duration
	HealthCheck time.Duration
	SSLMode     string
}

// BlacklistConfig represents blacklist settings
type BlacklistConfig struct {
	MaxFailedAttempts  int
	BlockDuration      time.Duration
	FailureExpiration  time.Duration
	SyncInterval       time.Duration
	MaxSyncBatchSize   int
	MaxCachedFailures  int
	MaxBlockedIPs      int
	MaxGreylistedIPs   int
	GreylistThreshold  int
	GreylistDuration   time.Duration
	WhitelistDuration  time.Duration
	MaxWhitelistedIPs  int
	IPReputationWeight float64
}

// DefaultDatabaseConfig returns the default database configuration
func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Host:        "localhost",
		Port:        5432,
		User:        "postgres",
		Password:    "postgres",
		Database:    "mycelium",
		MaxConns:    10,
		MaxIdleTime: time.Minute * 3,
		HealthCheck: time.Second * 5,
		SSLMode:     "disable",
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate network settings
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Port == 0 {
		return fmt.Errorf("port is required")
	}
	if c.P2PPort == 0 {
		return fmt.Errorf("P2P port is required")
	}
	if c.WebSocketPort == 0 {
		return fmt.Errorf("WebSocket port is required")
	}
	if c.NetworkID == "" {
		return fmt.Errorf("network ID is required")
	}
	if c.MaxPeers < 0 {
		return fmt.Errorf("max peers must be non-negative")
	}
	if c.ConnectionLimit < 0 {
		return fmt.Errorf("connection limit must be non-negative")
	}
	if c.ConnectionLimit < c.MaxPeers {
		return fmt.Errorf("connection limit (%d) must be greater than or equal to max peers (%d)",
			c.ConnectionLimit, c.MaxPeers)
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("listen address is required")
	}
	if c.SubstrateURL == "" {
		return fmt.Errorf("substrate URL is required")
	}
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.NetUID == 0 {
		return fmt.Errorf("network UID is required")
	}
	if c.Coldkey != "" && !isValidSS58Address(c.Coldkey) {
		return fmt.Errorf("invalid coldkey format: must be SS58 address")
	}

	// Validate node identity
	if c.Hotkey == "" {
		return fmt.Errorf("hotkey is required")
	}
	if c.NodeType == "" {
		return fmt.Errorf("node type is required")
	}
	if c.PublicKey == "" {
		return fmt.Errorf("public key is required")
	}
	if c.PrivateKey == "" {
		return fmt.Errorf("private key is required")
	}

	// Validate database settings
	if c.DatabaseURL == "" {
		return fmt.Errorf("database URL is required")
	}

	// Validate security settings
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT secret is required")
	}

	// Validate timeouts and intervals
	if c.ConnectionTimeout <= 0 {
		return fmt.Errorf("connection timeout must be positive")
	}
	if c.DialTimeout <= 0 {
		return fmt.Errorf("dial timeout must be positive")
	}
	if c.PingInterval <= 0 {
		return fmt.Errorf("ping interval must be positive")
	}
	if c.SyncInterval <= 0 {
		return fmt.Errorf("sync interval must be positive")
	}

	// Validate port range
	if c.PortRange[0] >= c.PortRange[1] {
		return fmt.Errorf("invalid port range: min port must be less than max port")
	}
	if c.PortRange[0] < MinPort || c.PortRange[1] > MaxPort {
		return fmt.Errorf("port range must be between %d and %d", MinPort, MaxPort)
	}

	// Validate minimum stake
	if c.MinStake == 0 {
		return fmt.Errorf("minimum stake must be greater than 0")
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

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	loader := NewEnvLoader(DefaultEnvPrefix)
	loader.LoadAll()

	cfg := &Config{}
	var err error

	// Network configuration
	cfg.Host = loader.GetString("HOST", "0.0.0.0")
	cfg.ListenAddr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	cfg.SubstrateURL = loader.GetString("SUBSTRATE_URL", "ws://localhost:9944")
	cfg.Version = loader.GetString("VERSION", "1.0.0")

	if cfg.Port, err = loader.GetUint16("PORT", 8080); err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}

	if cfg.P2PPort, err = loader.GetUint16("P2P_PORT", 9090); err != nil {
		return nil, fmt.Errorf("invalid P2P port: %w", err)
	}

	if cfg.WebSocketPort, err = loader.GetUint16("WS_PORT", 8081); err != nil {
		return nil, fmt.Errorf("invalid WebSocket port: %w", err)
	}

	cfg.NetworkID = loader.GetString("NETWORK_ID", "testnet")

	bootstrapNodes := loader.GetString("BOOTSTRAP_NODES", "")
	if bootstrapNodes != "" {
		cfg.BootstrapNodes = strings.Split(bootstrapNodes, ",")
	}

	if cfg.MaxPeers, err = loader.GetInt("MAX_PEERS", 50); err != nil {
		return nil, fmt.Errorf("invalid max peers: %w", err)
	}

	if cfg.ConnectionLimit, err = loader.GetInt("CONNECTION_LIMIT", 100); err != nil {
		return nil, fmt.Errorf("invalid connection limit: %w", err)
	}

	// Node identity
	if cfg.Hotkey, err = loader.GetStringValidated("HOTKEY", "", ValidateNotEmpty, ValidateSS58Address); err != nil {
		return nil, fmt.Errorf("invalid hotkey: %w", err)
	}

	if cfg.NodeType, err = loader.GetStringValidated("NODE_TYPE", "", ValidateNotEmpty); err != nil {
		return nil, fmt.Errorf("invalid node type: %w", err)
	}

	if cfg.PublicKey, err = loader.Required("PUBLIC_KEY"); err != nil {
		return nil, err
	}

	if cfg.PrivateKey, err = loader.Required("PRIVATE_KEY"); err != nil {
		return nil, err
	}

	// Database configuration
	if cfg.DatabaseURL, err = loader.Required("DATABASE_URL"); err != nil {
		return nil, err
	}

	// Security settings
	if cfg.JWTSecret, err = loader.Required("JWT_SECRET"); err != nil {
		return nil, err
	}

	// Timeouts and intervals
	if cfg.ConnectionTimeout, err = loader.GetDuration("CONNECTION_TIMEOUT", 30*time.Second); err != nil {
		return nil, fmt.Errorf("invalid connection timeout: %w", err)
	}

	if cfg.DialTimeout, err = loader.GetDuration("DIAL_TIMEOUT", 5*time.Second); err != nil {
		return nil, fmt.Errorf("invalid dial timeout: %w", err)
	}

	if cfg.PingInterval, err = loader.GetDuration("PING_INTERVAL", 30*time.Second); err != nil {
		return nil, fmt.Errorf("invalid ping interval: %w", err)
	}

	if cfg.SyncInterval, err = loader.GetDuration("SYNC_INTERVAL", 60*time.Second); err != nil {
		return nil, fmt.Errorf("invalid sync interval: %w", err)
	}

	// Port rotation settings
	if enabled := loader.GetString("PORT_ROTATION_ENABLED", ""); enabled != "" {
		cfg.PortRotation.Enabled = enabled == "true"
	}
	if interval := loader.GetString("PORT_ROTATION_INTERVAL", ""); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil {
			cfg.PortRotation.Interval = d
		}
	}
	if batchSize := loader.GetString("PORT_ROTATION_BATCH_SIZE", ""); batchSize != "" {
		if b, err := strconv.Atoi(batchSize); err == nil {
			cfg.PortRotation.BatchSize = b
		}
	}

	// Blacklist configuration
	if maxFailed := loader.GetString("BLACKLIST_MAX_FAILED", ""); maxFailed != "" {
		if m, err := strconv.Atoi(maxFailed); err == nil {
			cfg.Blacklist.MaxFailedAttempts = m
		}
	}
	if duration := loader.GetString("BLACKLIST_GREYLIST_DURATION", ""); duration != "" {
		if d, err := time.ParseDuration(duration); err == nil {
			cfg.Blacklist.GreylistDuration = d
		}
	}
	if threshold := loader.GetString("BLACKLIST_THRESHOLD", ""); threshold != "" {
		if t, err := strconv.Atoi(threshold); err == nil {
			cfg.Blacklist.GreylistThreshold = t
		}
	}
	if minStake := loader.GetString("BLACKLIST_MIN_STAKE", ""); minStake != "" {
		if s, err := strconv.ParseFloat(minStake, 64); err == nil {
			cfg.Blacklist.IPReputationWeight = s
		}
	}

	// Verify URL
	cfg.VerifyURL = loader.GetString("VERIFY_URL", "http://localhost:8081")

	// Port range configuration
	minPort, err := loader.GetUint16("PORT_RANGE_MIN", WSPortRangeMin)
	if err != nil {
		return nil, fmt.Errorf("invalid port range min: %w", err)
	}
	maxPort, err := loader.GetUint16("PORT_RANGE_MAX", WSPortRangeMax)
	if err != nil {
		return nil, fmt.Errorf("invalid port range max: %w", err)
	}
	cfg.PortRange = [2]uint16{minPort, maxPort}

	// Minimum stake configuration
	if stake := loader.GetString("MIN_STAKE", ""); stake != "" {
		if s, err := strconv.ParseUint(stake, 10, 64); err == nil {
			cfg.MinStake = s
		}
	} else {
		cfg.MinStake = uint64(DefaultMinStake)
	}

	// Network configuration
	if uid := loader.GetString("NET_UID", ""); uid != "" {
		if n, err := strconv.ParseUint(uid, 10, 16); err == nil {
			cfg.NetUID = uint16(n)
		}
	}

	if coldkey, err := loader.GetStringValidated("COLDKEY", "", ValidateNotEmpty, ValidateSS58Address); err == nil {
		cfg.Coldkey = coldkey
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func (c *DatabaseConfig) ToDBConfig() database.Config {
	return database.Config{
		Host:        c.Host,
		Port:        int(c.Port),
		User:        c.User,
		Password:    c.Password,
		Database:    c.Database,
		MaxConns:    int32(c.MaxConns),
		MaxIdleTime: c.MaxIdleTime,
		HealthCheck: c.HealthCheck,
		SSLMode:     c.SSLMode,
	}
}

// ToDBConfig converts the DatabaseURL to a database.Config
func (c *Config) ToDBConfig() database.Config {
	// Parse the database URL and return a config
	// Example URL format: postgres://username:password@localhost:5432/dbname?sslmode=disable
	return database.Config{
		Host:        "localhost", // These should be parsed from DatabaseURL
		Port:        5432,
		User:        "postgres",
		Password:    "postgres",
		Database:    "mycelium",
		MaxConns:    10,
		MaxIdleTime: time.Minute * 3,
		HealthCheck: time.Second * 5,
		SSLMode:     "disable",
	}
}
