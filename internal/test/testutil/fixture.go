package testutil

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestFixture represents a test fixture
type TestFixture struct {
	t    *testing.T
	dir  string
	data map[string]interface{}
}

// NewTestFixture creates a new test fixture
func NewTestFixture(t *testing.T) *TestFixture {
	dir := CreateTestDir(t)
	return &TestFixture{
		t:    t,
		dir:  dir,
		data: make(map[string]interface{}),
	}
}

// Dir returns the fixture directory
func (f *TestFixture) Dir() string {
	return f.dir
}

// Load loads fixture data from a file
func (f *TestFixture) Load(name string) error {
	path := filepath.Join(f.dir, name)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read fixture: %w", err)
	}

	ext := filepath.Ext(name)
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &f.data); err != nil {
			return fmt.Errorf("failed to parse JSON fixture: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &f.data); err != nil {
			return fmt.Errorf("failed to parse YAML fixture: %w", err)
		}
	default:
		return fmt.Errorf("unsupported fixture format: %s", ext)
	}

	return nil
}

// Save saves fixture data to a file
func (f *TestFixture) Save(name string) error {
	path := filepath.Join(f.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	var data []byte
	var err error

	ext := filepath.Ext(name)
	switch ext {
	case ".json":
		data, err = json.MarshalIndent(f.data, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON fixture: %w", err)
		}
	case ".yaml", ".yml":
		data, err = yaml.Marshal(f.data)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML fixture: %w", err)
		}
	default:
		return fmt.Errorf("unsupported fixture format: %s", ext)
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write fixture: %w", err)
	}

	return nil
}

// Get gets a value from the fixture
func (f *TestFixture) Get(key string) interface{} {
	return f.data[key]
}

// Set sets a value in the fixture
func (f *TestFixture) Set(key string, value interface{}) {
	f.data[key] = value
}

// GetString gets a string value from the fixture
func (f *TestFixture) GetString(key string) string {
	value, ok := f.data[key].(string)
	if !ok {
		f.t.Fatalf("Expected string value for key %s", key)
	}
	return value
}

// GetInt gets an integer value from the fixture
func (f *TestFixture) GetInt(key string) int {
	value, ok := f.data[key].(int)
	if !ok {
		f.t.Fatalf("Expected integer value for key %s", key)
	}
	return value
}

// GetFloat gets a float value from the fixture
func (f *TestFixture) GetFloat(key string) float64 {
	value, ok := f.data[key].(float64)
	if !ok {
		f.t.Fatalf("Expected float value for key %s", key)
	}
	return value
}

// GetBool gets a boolean value from the fixture
func (f *TestFixture) GetBool(key string) bool {
	value, ok := f.data[key].(bool)
	if !ok {
		f.t.Fatalf("Expected boolean value for key %s", key)
	}
	return value
}

// GetMap gets a map value from the fixture
func (f *TestFixture) GetMap(key string) map[string]interface{} {
	value, ok := f.data[key].(map[string]interface{})
	if !ok {
		f.t.Fatalf("Expected map value for key %s", key)
	}
	return value
}

// GetSlice gets a slice value from the fixture
func (f *TestFixture) GetSlice(key string) []interface{} {
	value, ok := f.data[key].([]interface{})
	if !ok {
		f.t.Fatalf("Expected slice value for key %s", key)
	}
	return value
}

// RequireKey asserts that a key exists in the fixture
func (f *TestFixture) RequireKey(key string) {
	_, ok := f.data[key]
	require.True(f.t, ok, "Key not found: %s", key)
}

// RequireNoKey asserts that a key does not exist in the fixture
func (f *TestFixture) RequireNoKey(key string) {
	_, ok := f.data[key]
	require.False(f.t, ok, "Unexpected key found: %s", key)
}

// RequireValue asserts that a key has a specific value
func (f *TestFixture) RequireValue(key string, expected interface{}) {
	value := f.Get(key)
	require.Equal(f.t, expected, value)
}

// WithTestFixture runs a test with a fixture
func WithTestFixture(t *testing.T, fn func(*TestFixture)) {
	fixture := NewTestFixture(t)
	fn(fixture)
}

// TestFixtureBuilder helps build test fixtures
type TestFixtureBuilder struct {
	t       *testing.T
	fixture *TestFixture
}

// NewTestFixtureBuilder creates a new fixture builder
func NewTestFixtureBuilder(t *testing.T) *TestFixtureBuilder {
	return &TestFixtureBuilder{
		t:       t,
		fixture: NewTestFixture(t),
	}
}

// WithValue adds a value to the fixture
func (b *TestFixtureBuilder) WithValue(key string, value interface{}) *TestFixtureBuilder {
	b.fixture.Set(key, value)
	return b
}

// WithFile adds a fixture file
func (b *TestFixtureBuilder) WithFile(name string, data interface{}) *TestFixtureBuilder {
	path := filepath.Join(b.fixture.Dir(), name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		b.t.Fatalf("Failed to create directory: %v", err)
	}

	var fileData []byte
	var err error

	ext := filepath.Ext(name)
	switch ext {
	case ".json":
		fileData, err = json.MarshalIndent(data, "", "  ")
	case ".yaml", ".yml":
		fileData, err = yaml.Marshal(data)
	default:
		b.t.Fatalf("Unsupported fixture format: %s", ext)
	}

	if err != nil {
		b.t.Fatalf("Failed to marshal fixture data: %v", err)
	}

	if err := ioutil.WriteFile(path, fileData, 0644); err != nil {
		b.t.Fatalf("Failed to write fixture file: %v", err)
	}

	return b
}

// Build returns the built fixture
func (b *TestFixtureBuilder) Build() *TestFixture {
	return b.fixture
}
