// Package database provides database connectivity and operations
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Database defines the interface for database operations
type Database interface {
	Ping(ctx context.Context) error
	Close() error
	WithTx(ctx context.Context, fn func(pgx.Tx) error) error
	GetPool() *pgxpool.Pool
}

// Config holds database connection configuration
type Config struct {
	Host        string
	Port        int
	User        string
	Password    string
	Database    string
	MaxConns    int32
	MaxIdleTime time.Duration
	HealthCheck time.Duration
	SSLMode     string
}

// PostgresDB represents a database connection pool
type PostgresDB struct {
	pool *pgxpool.Pool
	cfg  Config
}

// New creates a new database connection pool
func New(cfg Config) (Database, error) {
	if cfg.MaxConns == 0 {
		cfg.MaxConns = 10
	}
	if cfg.MaxIdleTime == 0 {
		cfg.MaxIdleTime = time.Minute * 3
	}
	if cfg.HealthCheck == 0 {
		cfg.HealthCheck = time.Second * 5
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}

	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s&pool_max_conns=%d&pool_max_conn_idle_time=%v",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.SSLMode,
		cfg.MaxConns,
		cfg.MaxIdleTime,
	)

	poolCfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("error parsing connection string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("error creating connection pool: %w", err)
	}

	db := &PostgresDB{
		pool: pool,
		cfg:  cfg,
	}

	// Verify connection
	if err := db.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	// Start health check
	go db.startHealthCheck()

	return db, nil
}

// Ping verifies database connectivity
func (db *PostgresDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Close closes the database connection pool
func (db *PostgresDB) Close() error {
	db.pool.Close()
	return nil
}

// GetPool returns the underlying connection pool
func (db *PostgresDB) GetPool() *pgxpool.Pool {
	return db.pool
}

// startHealthCheck periodically checks database connectivity
func (db *PostgresDB) startHealthCheck() {
	ticker := time.NewTicker(db.cfg.HealthCheck)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		if err := db.Ping(ctx); err != nil {
			// TODO: Implement proper error handling/logging
			fmt.Printf("Database health check failed: %v\n", err)
		}
		cancel()
	}
}

// WithTx executes a function within a transaction
func (db *PostgresDB) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("error rolling back transaction: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

// Validate checks if the database configuration is valid
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("database host is required")
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", c.Port)
	}

	if c.User == "" {
		return fmt.Errorf("database user is required")
	}

	if c.Database == "" {
		return fmt.Errorf("database name is required")
	}

	if c.MaxConns < 1 {
		return fmt.Errorf("max connections must be at least 1")
	}

	if c.MaxIdleTime < time.Second {
		return fmt.Errorf("max idle time must be at least 1 second")
	}

	if c.HealthCheck < time.Second {
		return fmt.Errorf("health check interval must be at least 1 second")
	}

	validSSLModes := map[string]bool{
		"disable":     true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}

	if !validSSLModes[c.SSLMode] {
		return fmt.Errorf("invalid SSL mode: %s", c.SSLMode)
	}

	return nil
}
