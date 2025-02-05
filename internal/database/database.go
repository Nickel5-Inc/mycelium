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

// Config holds database configuration
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

// PostgresDB implements the Database interface
type PostgresDB struct {
	pool *pgxpool.Pool
	cfg  Config
}

// New creates a new database connection
func New(cfg Config) (Database, error) {
	poolConfig, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}

	// Configure the pool
	poolConfig.ConnConfig.Host = cfg.Host
	poolConfig.ConnConfig.Port = uint16(cfg.Port)
	poolConfig.ConnConfig.User = cfg.User
	poolConfig.ConnConfig.Password = cfg.Password
	poolConfig.ConnConfig.Database = cfg.Database

	// Set pool options
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MaxConnIdleTime = cfg.MaxIdleTime

	// Connect to database
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	db := &PostgresDB{
		pool: pool,
		cfg:  cfg,
	}

	// Start health check if configured
	if cfg.HealthCheck > 0 {
		go db.startHealthCheck()
	}

	return db, nil
}

// Ping checks database connectivity
func (db *PostgresDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Close closes the database connection
func (db *PostgresDB) Close() error {
	db.pool.Close()
	return nil
}

// GetPool returns the connection pool
func (db *PostgresDB) GetPool() *pgxpool.Pool {
	return db.pool
}

// startHealthCheck starts periodic health checks
func (db *PostgresDB) startHealthCheck() {
	ticker := time.NewTicker(db.cfg.HealthCheck)
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := db.Ping(ctx)
		if err != nil {
			// TODO: Add proper logging/metrics
			fmt.Printf("Database health check failed: %v\n", err)
		}
		cancel()
	}
}

// WithTx executes a function within a transaction
func (db *PostgresDB) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rolling back transaction: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// Validate validates the database configuration
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Port <= 0 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.User == "" {
		return fmt.Errorf("user is required")
	}
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	if c.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if c.MaxConns <= 0 {
		return fmt.Errorf("invalid max connections: %d", c.MaxConns)
	}
	return nil
}
