// Package testutil provides essential testing utilities
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"mycelium/internal/config"
	"mycelium/internal/database"
	"mycelium/internal/identity"

	"github.com/stretchr/testify/require"
)

// TestConfig returns a test configuration
func TestConfig() *config.Config {
	return &config.Config{
		Host:        "localhost",
		Port:        8080,
		NetUID:      1,
		Hotkey:      "test",
		Coldkey:     "test",
		MinStake:    1000,
		DatabaseURL: "postgres://postgres:postgres@localhost:5432/mycelium_test?sslmode=disable",
	}
}

// TestDatabase returns a test database connection
func TestDatabase(t *testing.T) database.Database {
	db, err := database.New(database.Config{
		Host:        "localhost",
		Port:        5432,
		User:        "postgres",
		Password:    "postgres",
		Database:    "mycelium_test",
		SSLMode:     "disable",
		MaxConns:    10,
		MaxIdleTime: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Clean database
	err = db.WithTx(context.Background(), func(tx pgx.Tx) error {
		// Drop all tables
		_, err := tx.Exec(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
		return err
	})
	if err != nil {
		t.Fatalf("Failed to clean test database: %v", err)
	}

	return db
}

// TestIdentity returns a test identity
func TestIdentity(t *testing.T) *identity.Identity {
	return identity.New(nil, 1)
}

// CreateTestDir creates a temporary test directory
func CreateTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mycelium-test-*")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// WithTestDir creates a temporary test directory and runs the function with it
func WithTestDir(t *testing.T, fn func(dir string)) {
	dir := CreateTestDir(t)
	fn(dir)
}

// RequireFileExists fails the test if the file doesn't exist
func RequireFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("File does not exist: %s", path)
	}
}

// RequireFileContains asserts that a file contains text
func RequireFileContains(t *testing.T, path string, expected string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read file: %v", err)
		return
	}
	if !strings.Contains(string(data), expected) {
		t.Errorf("File %s does not contain expected text: %s", path, expected)
	}
}

// WaitForCondition waits for a condition to be true with timeout
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Condition not met within timeout: %s", msg)
}

// CreateTestFile creates a file with given content in test directory
func CreateTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	return path
}

// RequireNoError fails the test if err is not nil
func RequireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// RequireError asserts specific error
func RequireError(t *testing.T, err error, expected error) {
	require.Error(t, err)
	require.Equal(t, expected, err)
}

// RequireErrorContains asserts error contains string
func RequireErrorContains(t *testing.T, err error, expected string) {
	require.Error(t, err)
	require.Contains(t, err.Error(), expected)
}

// RequireErrorIs asserts error matches target
func RequireErrorIs(t *testing.T, err error, target error) {
	require.ErrorIs(t, err, target)
}

// RequireErrorAs asserts error type
func RequireErrorAs(t *testing.T, err error, target interface{}) {
	require.ErrorAs(t, err, target)
}

// RequirePanic asserts function panics
func RequirePanic(t *testing.T, fn func()) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "Expected panic")
	}()
	fn()
}

// RequirePanicMatch asserts panic value
func RequirePanicMatch(t *testing.T, fn func(), expected interface{}) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "Expected panic")
		require.Equal(t, expected, r)
	}()
	fn()
}

// MockSubstrateURL returns a mock substrate URL for testing
func MockSubstrateURL() string {
	return "ws://localhost:9944"
}
