package testutil

import (
	"os"
	"sync"
	"testing"
)

// TestCleanup represents a test cleanup manager
type TestCleanup struct {
	t        *testing.T
	mu       sync.Mutex
	cleanups []func() error
	dirs     []string
	files    []string
}

// NewTestCleanup creates a new test cleanup manager
func NewTestCleanup(t *testing.T) *TestCleanup {
	cleanup := &TestCleanup{
		t:        t,
		cleanups: make([]func() error, 0),
		dirs:     make([]string, 0),
		files:    make([]string, 0),
	}

	t.Cleanup(func() {
		cleanup.Run()
	})

	return cleanup
}

// AddCleanup adds a cleanup function
func (c *TestCleanup) AddCleanup(fn func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanups = append(c.cleanups, fn)
}

// AddDir adds a directory to cleanup
func (c *TestCleanup) AddDir(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dirs = append(c.dirs, dir)
}

// AddFile adds a file to cleanup
func (c *TestCleanup) AddFile(file string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files = append(c.files, file)
}

// Run runs all cleanup operations
func (c *TestCleanup) Run() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Run cleanup functions in reverse order
	for i := len(c.cleanups) - 1; i >= 0; i-- {
		if err := c.cleanups[i](); err != nil {
			c.t.Logf("Cleanup error: %v", err)
		}
	}

	// Remove files
	for _, file := range c.files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			c.t.Logf("Failed to remove file %s: %v", file, err)
		}
	}

	// Remove directories in reverse order
	for i := len(c.dirs) - 1; i >= 0; i-- {
		dir := c.dirs[i]
		if err := os.RemoveAll(dir); err != nil {
			c.t.Logf("Failed to remove directory %s: %v", dir, err)
		}
	}
}

// WithTestCleanup runs a test with cleanup
func WithTestCleanup(t *testing.T, fn func(*TestCleanup)) {
	cleanup := NewTestCleanup(t)
	fn(cleanup)
}

// TestCleanupBuilder helps build test cleanup
type TestCleanupBuilder struct {
	t       *testing.T
	cleanup *TestCleanup
}

// NewTestCleanupBuilder creates a new cleanup builder
func NewTestCleanupBuilder(t *testing.T) *TestCleanupBuilder {
	return &TestCleanupBuilder{
		t:       t,
		cleanup: NewTestCleanup(t),
	}
}

// WithCleanup adds a cleanup function
func (b *TestCleanupBuilder) WithCleanup(fn func() error) *TestCleanupBuilder {
	b.cleanup.AddCleanup(fn)
	return b
}

// WithDir adds a directory to cleanup
func (b *TestCleanupBuilder) WithDir(dir string) *TestCleanupBuilder {
	b.cleanup.AddDir(dir)
	return b
}

// WithFile adds a file to cleanup
func (b *TestCleanupBuilder) WithFile(file string) *TestCleanupBuilder {
	b.cleanup.AddFile(file)
	return b
}

// WithTempDir creates and adds a temporary directory to cleanup
func (b *TestCleanupBuilder) WithTempDir(prefix string) (*TestCleanupBuilder, string) {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		b.t.Fatalf("Failed to create temp dir: %v", err)
	}
	b.cleanup.AddDir(dir)
	return b, dir
}

// WithTempFile creates and adds a temporary file to cleanup
func (b *TestCleanupBuilder) WithTempFile(dir, pattern string) (*TestCleanupBuilder, string) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		b.t.Fatalf("Failed to create temp file: %v", err)
	}
	f.Close()
	b.cleanup.AddFile(f.Name())
	return b, f.Name()
}

// Build returns the built cleanup
func (b *TestCleanupBuilder) Build() *TestCleanup {
	return b.cleanup
}

// CleanupOnError runs cleanup if an error occurs
func CleanupOnError(t *testing.T, err error, cleanup func()) {
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
}

// CleanupAll runs multiple cleanup functions
func CleanupAll(t *testing.T, cleanups ...func() error) {
	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := cleanups[i](); err != nil {
			t.Logf("Cleanup error: %v", err)
		}
	}
}

// CleanupFiles removes multiple files
func CleanupFiles(t *testing.T, files ...string) {
	for _, file := range files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			t.Logf("Failed to remove file %s: %v", file, err)
		}
	}
}

// CleanupDirs removes multiple directories
func CleanupDirs(t *testing.T, dirs ...string) {
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.RemoveAll(dirs[i]); err != nil {
			t.Logf("Failed to remove directory %s: %v", dirs[i], err)
		}
	}
}
