package testutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStorage represents a test storage
type TestStorage struct {
	t    *testing.T
	dir  string
	data map[string][]byte
}

// NewTestStorage creates a new test storage
func NewTestStorage(t *testing.T) *TestStorage {
	dir := CreateTestDir(t)
	return &TestStorage{
		t:    t,
		dir:  dir,
		data: make(map[string][]byte),
	}
}

// Dir returns the storage directory
func (s *TestStorage) Dir() string {
	return s.dir
}

// Write writes data to storage
func (s *TestStorage) Write(key string, data []byte) error {
	path := filepath.Join(s.dir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	s.data[key] = data
	return nil
}

// Read reads data from storage
func (s *TestStorage) Read(key string) ([]byte, error) {
	path := filepath.Join(s.dir, key)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return data, nil
}

// Delete deletes data from storage
func (s *TestStorage) Delete(key string) error {
	path := filepath.Join(s.dir, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	delete(s.data, key)
	return nil
}

// List lists all keys in storage
func (s *TestStorage) List() ([]string, error) {
	var keys []string
	err := filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, err := filepath.Rel(s.dir, path)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}
			keys = append(keys, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}
	return keys, nil
}

// Clear clears all data from storage
func (s *TestStorage) Clear() error {
	if err := os.RemoveAll(s.dir); err != nil {
		return fmt.Errorf("failed to remove directory: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	s.data = make(map[string][]byte)
	return nil
}

// RequireExists asserts that a key exists in storage
func (s *TestStorage) RequireExists(key string) {
	path := filepath.Join(s.dir, key)
	_, err := os.Stat(path)
	require.NoError(s.t, err)
}

// RequireNotExists asserts that a key does not exist in storage
func (s *TestStorage) RequireNotExists(key string) {
	path := filepath.Join(s.dir, key)
	_, err := os.Stat(path)
	require.True(s.t, os.IsNotExist(err))
}

// RequireData asserts that a key contains expected data
func (s *TestStorage) RequireData(key string, expected []byte) {
	data, err := s.Read(key)
	require.NoError(s.t, err)
	require.Equal(s.t, expected, data)
}

// RequireNoData asserts that a key contains no data
func (s *TestStorage) RequireNoData(key string) {
	data, err := s.Read(key)
	require.NoError(s.t, err)
	require.Nil(s.t, data)
}

// WithTestStorage runs a test with storage
func WithTestStorage(t *testing.T, fn func(*TestStorage)) {
	storage := NewTestStorage(t)
	fn(storage)
}

// TestStorageBuilder helps build test storage
type TestStorageBuilder struct {
	t       *testing.T
	storage *TestStorage
}

// NewTestStorageBuilder creates a new storage builder
func NewTestStorageBuilder(t *testing.T) *TestStorageBuilder {
	return &TestStorageBuilder{
		t:       t,
		storage: NewTestStorage(t),
	}
}

// WithData adds data to storage
func (b *TestStorageBuilder) WithData(key string, data []byte) *TestStorageBuilder {
	err := b.storage.Write(key, data)
	require.NoError(b.t, err)
	return b
}

// WithFile adds a file to storage
func (b *TestStorageBuilder) WithFile(key string, content string) *TestStorageBuilder {
	err := b.storage.Write(key, []byte(content))
	require.NoError(b.t, err)
	return b
}

// WithDir adds a directory to storage
func (b *TestStorageBuilder) WithDir(key string) *TestStorageBuilder {
	path := filepath.Join(b.storage.Dir(), key)
	err := os.MkdirAll(path, 0755)
	require.NoError(b.t, err)
	return b
}

// Build returns the built storage
func (b *TestStorageBuilder) Build() *TestStorage {
	return b.storage
}
