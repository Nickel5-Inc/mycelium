package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/require"
)

// TestDBConn represents a test database connection
type TestDBConn struct {
	t      *testing.T
	pool   *pgxpool.Pool
	dbName string
}

// NewTestDBConn creates a new test database connection
func NewTestDBConn(t *testing.T) *TestDBConn {
	dbName := fmt.Sprintf("mycelium_test_%d", nextDBID())
	connStr := fmt.Sprintf("postgres://postgres:postgres@localhost:5432/%s", dbName)

	// Connect to postgres to create test database
	pool, err := pgxpool.Connect(context.Background(), "postgres://postgres:postgres@localhost:5432/postgres")
	require.NoError(t, err)
	defer pool.Close()

	// Create test database
	_, err = pool.Exec(context.Background(), fmt.Sprintf("CREATE DATABASE %s", dbName))
	require.NoError(t, err)

	// Connect to test database
	pool, err = pgxpool.Connect(context.Background(), connStr)
	require.NoError(t, err)

	db := &TestDBConn{
		t:      t,
		pool:   pool,
		dbName: dbName,
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// Pool returns the underlying connection pool
func (db *TestDBConn) Pool() *pgxpool.Pool {
	return db.pool
}

// Close closes the database connection and drops the test database
func (db *TestDBConn) Close() {
	if db.pool != nil {
		db.pool.Close()

		// Connect to postgres to drop test database
		pool, err := pgxpool.Connect(context.Background(), "postgres://postgres:postgres@localhost:5432/postgres")
		if err == nil {
			defer pool.Close()
			_, _ = pool.Exec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", db.dbName))
		}
	}
}

// WithTestDBConn runs a test with a database connection
func WithTestDBConn(t *testing.T, fn func(*TestDBConn)) {
	db := NewTestDBConn(t)
	fn(db)
}

// ExecSQL executes a SQL statement
func (db *TestDBConn) ExecSQL(sql string) {
	_, err := db.pool.Exec(context.Background(), sql)
	require.NoError(db.t, err)
}

// QueryRow executes a query that returns a single row
func (db *TestDBConn) QueryRow(sql string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(context.Background(), sql, args...)
}

// Query executes a query that returns multiple rows
func (db *TestDBConn) Query(sql string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(context.Background(), sql, args...)
}

// Begin starts a new transaction
func (db *TestDBConn) Begin() (pgx.Tx, error) {
	return db.pool.Begin(context.Background())
}

// WithTx runs a function within a transaction
func (db *TestDBConn) WithTx(fn func(pgx.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(context.Background())
}

// RequireRowCount asserts the number of rows in a table
func (db *TestDBConn) RequireRowCount(table string, expected int) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
	require.NoError(db.t, err)
	require.Equal(db.t, expected, count)
}

// RequireRowExists asserts that a row exists
func (db *TestDBConn) RequireRowExists(sql string, args ...interface{}) {
	var exists bool
	err := db.QueryRow(sql, args...).Scan(&exists)
	require.NoError(db.t, err)
	require.True(db.t, exists)
}

// RequireRowNotExists asserts that a row does not exist
func (db *TestDBConn) RequireRowNotExists(sql string, args ...interface{}) {
	var exists bool
	err := db.QueryRow(sql, args...).Scan(&exists)
	require.NoError(db.t, err)
	require.False(db.t, exists)
}

var dbID int32

// nextDBID returns the next database ID
func nextDBID() int32 {
	dbID++
	return dbID
}
