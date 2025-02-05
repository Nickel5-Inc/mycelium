package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// RetryConfig represents retry configuration
type RetryConfig struct {
	Attempts  int
	Delay     time.Duration
	MaxDelay  time.Duration
	Timeout   time.Duration
	Backoff   float64
	OnRetry   func(attempt int, err error)
	OnTimeout func(attempts int, lastErr error)
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		Attempts:  3,
		Delay:     100 * time.Millisecond,
		MaxDelay:  5 * time.Second,
		Timeout:   30 * time.Second,
		Backoff:   2.0,
		OnRetry:   nil,
		OnTimeout: nil,
	}
}

// Retry retries a function until it succeeds or times out
func Retry(t *testing.T, config RetryConfig, fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	var lastErr error
	attempt := 0
	delay := config.Delay

	for {
		select {
		case <-ctx.Done():
			if config.OnTimeout != nil {
				config.OnTimeout(attempt, lastErr)
			}
			return fmt.Errorf("timeout after %d attempts: %v", attempt, lastErr)
		default:
			if attempt >= config.Attempts {
				if config.OnTimeout != nil {
					config.OnTimeout(attempt, lastErr)
				}
				return fmt.Errorf("max attempts (%d) reached: %v", config.Attempts, lastErr)
			}

			err := fn()
			if err == nil {
				return nil
			}

			lastErr = err
			attempt++

			if config.OnRetry != nil {
				config.OnRetry(attempt, err)
			}

			if attempt < config.Attempts {
				time.Sleep(delay)
				delay = time.Duration(float64(delay) * config.Backoff)
				if delay > config.MaxDelay {
					delay = config.MaxDelay
				}
			}
		}
	}
}

// RetryWithContext retries a function with context until it succeeds or times out
func RetryWithContext(ctx context.Context, t *testing.T, config RetryConfig, fn func(context.Context) error) error {
	var lastErr error
	attempt := 0
	delay := config.Delay

	for {
		select {
		case <-ctx.Done():
			if config.OnTimeout != nil {
				config.OnTimeout(attempt, lastErr)
			}
			return fmt.Errorf("context cancelled after %d attempts: %v", attempt, lastErr)
		default:
			if attempt >= config.Attempts {
				if config.OnTimeout != nil {
					config.OnTimeout(attempt, lastErr)
				}
				return fmt.Errorf("max attempts (%d) reached: %v", config.Attempts, lastErr)
			}

			err := fn(ctx)
			if err == nil {
				return nil
			}

			lastErr = err
			attempt++

			if config.OnRetry != nil {
				config.OnRetry(attempt, err)
			}

			if attempt < config.Attempts {
				time.Sleep(delay)
				delay = time.Duration(float64(delay) * config.Backoff)
				if delay > config.MaxDelay {
					delay = config.MaxDelay
				}
			}
		}
	}
}

// RequireRetry asserts that a function succeeds within retries
func RequireRetry(t *testing.T, config RetryConfig, fn func() error) {
	err := Retry(t, config, fn)
	require.NoError(t, err)
}

// RequireRetryWithContext asserts that a function succeeds within retries with context
func RequireRetryWithContext(ctx context.Context, t *testing.T, config RetryConfig, fn func(context.Context) error) {
	err := RetryWithContext(ctx, t, config, fn)
	require.NoError(t, err)
}

// RetryBuilder helps build retry configuration
type RetryBuilder struct {
	t      *testing.T
	config RetryConfig
}

// NewRetryBuilder creates a new retry builder
func NewRetryBuilder(t *testing.T) *RetryBuilder {
	return &RetryBuilder{
		t:      t,
		config: DefaultRetryConfig(),
	}
}

// WithAttempts sets the number of attempts
func (b *RetryBuilder) WithAttempts(attempts int) *RetryBuilder {
	b.config.Attempts = attempts
	return b
}

// WithDelay sets the initial delay
func (b *RetryBuilder) WithDelay(delay time.Duration) *RetryBuilder {
	b.config.Delay = delay
	return b
}

// WithMaxDelay sets the maximum delay
func (b *RetryBuilder) WithMaxDelay(maxDelay time.Duration) *RetryBuilder {
	b.config.MaxDelay = maxDelay
	return b
}

// WithTimeout sets the timeout
func (b *RetryBuilder) WithTimeout(timeout time.Duration) *RetryBuilder {
	b.config.Timeout = timeout
	return b
}

// WithBackoff sets the backoff factor
func (b *RetryBuilder) WithBackoff(backoff float64) *RetryBuilder {
	b.config.Backoff = backoff
	return b
}

// WithOnRetry sets the retry callback
func (b *RetryBuilder) WithOnRetry(onRetry func(attempt int, err error)) *RetryBuilder {
	b.config.OnRetry = onRetry
	return b
}

// WithOnTimeout sets the timeout callback
func (b *RetryBuilder) WithOnTimeout(onTimeout func(attempts int, lastErr error)) *RetryBuilder {
	b.config.OnTimeout = onTimeout
	return b
}

// Build returns the built retry configuration
func (b *RetryBuilder) Build() RetryConfig {
	return b.config
}

// Retry retries a function with the built configuration
func (b *RetryBuilder) Retry(fn func() error) error {
	return Retry(b.t, b.config, fn)
}

// RetryWithContext retries a function with context and the built configuration
func (b *RetryBuilder) RetryWithContext(ctx context.Context, fn func(context.Context) error) error {
	return RetryWithContext(ctx, b.t, b.config, fn)
}

// RequireRetry asserts that a function succeeds with the built configuration
func (b *RetryBuilder) RequireRetry(fn func() error) {
	RequireRetry(b.t, b.config, fn)
}

// RequireRetryWithContext asserts that a function succeeds with context and the built configuration
func (b *RetryBuilder) RequireRetryWithContext(ctx context.Context, fn func(context.Context) error) {
	RequireRetryWithContext(ctx, b.t, b.config, fn)
}
