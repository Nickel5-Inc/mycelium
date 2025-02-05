package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestContext represents a test context
type TestContext struct {
	t       *testing.T
	ctx     context.Context
	cancel  context.CancelFunc
	timeout time.Duration
}

// NewTestContext creates a new test context
func NewTestContext(t *testing.T) *TestContext {
	return NewTestContextWithTimeout(t, 5*time.Second)
}

// NewTestContextWithTimeout creates a new test context with timeout
func NewTestContextWithTimeout(t *testing.T, timeout time.Duration) *TestContext {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(func() {
		cancel()
	})

	return &TestContext{
		t:       t,
		ctx:     ctx,
		cancel:  cancel,
		timeout: timeout,
	}
}

// Context returns the underlying context
func (c *TestContext) Context() context.Context {
	return c.ctx
}

// Cancel cancels the context
func (c *TestContext) Cancel() {
	c.cancel()
}

// Timeout returns the context timeout
func (c *TestContext) Timeout() time.Duration {
	return c.timeout
}

// RequireNotDone asserts that the context is not done
func (c *TestContext) RequireNotDone(t *testing.T) {
	select {
	case <-c.ctx.Done():
		t.Error("Context is done")
	default:
	}
}

// RequireDone asserts that the context is done
func (c *TestContext) RequireDone(t *testing.T) {
	select {
	case <-c.ctx.Done():
	default:
		t.Error("Context is not done")
	}
}

// RequireNoError asserts that the context has no error
func (c *TestContext) RequireNoError(t *testing.T) {
	require.NoError(t, c.ctx.Err())
}

// RequireError asserts that the context has an error
func (c *TestContext) RequireError(t *testing.T, expected error) {
	require.Equal(t, expected, c.ctx.Err())
}

// WithTestContext runs a test with a context
func WithTestContext(t *testing.T, fn func(*TestContext)) {
	ctx := NewTestContext(t)
	fn(ctx)
}

// WithTestContextTimeout runs a test with a context with timeout
func WithTestContextTimeout(t *testing.T, timeout time.Duration, fn func(*TestContext)) {
	ctx := NewTestContextWithTimeout(t, timeout)
	fn(ctx)
}

// TestContextBuilder helps build test contexts
type TestContextBuilder struct {
	t       *testing.T
	timeout time.Duration
	values  map[interface{}]interface{}
}

// NewTestContextBuilder creates a new context builder
func NewTestContextBuilder(t *testing.T) *TestContextBuilder {
	return &TestContextBuilder{
		t:       t,
		timeout: 5 * time.Second,
		values:  make(map[interface{}]interface{}),
	}
}

// WithTimeout sets the context timeout
func (b *TestContextBuilder) WithTimeout(timeout time.Duration) *TestContextBuilder {
	b.timeout = timeout
	return b
}

// WithValue adds a value to the context
func (b *TestContextBuilder) WithValue(key, value interface{}) *TestContextBuilder {
	b.values[key] = value
	return b
}

// Build returns the built context
func (b *TestContextBuilder) Build() *TestContext {
	ctx := context.Background()
	for k, v := range b.values {
		ctx = context.WithValue(ctx, k, v)
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	b.t.Cleanup(func() {
		cancel()
	})

	return &TestContext{
		t:       b.t,
		ctx:     ctx,
		cancel:  cancel,
		timeout: b.timeout,
	}
}
