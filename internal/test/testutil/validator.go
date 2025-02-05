package testutil

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestValidator represents a test validator node
type TestValidator struct {
	t          *testing.T
	id         string
	stake      float64
	server     *TestTCPServer
	metrics    *TestMetrics
	isActive   bool
	lastUpdate time.Time
}

// NewTestValidator creates a new test validator
func NewTestValidator(t *testing.T, id string, stake float64) *TestValidator {
	metrics := NewTestMetrics(t)

	validator := &TestValidator{
		t:          t,
		id:         id,
		stake:      stake,
		metrics:    metrics,
		isActive:   true,
		lastUpdate: time.Now(),
	}

	// Create TCP server for validator
	server := NewTestTCPServer(t, validator.handleConnection)
	validator.server = server

	return validator
}

// ID returns the validator ID
func (v *TestValidator) ID() string {
	return v.id
}

// Stake returns the validator stake
func (v *TestValidator) Stake() float64 {
	return v.stake
}

// SetStake sets the validator stake
func (v *TestValidator) SetStake(stake float64) {
	v.stake = stake
	v.lastUpdate = time.Now()
}

// IsActive returns whether the validator is active
func (v *TestValidator) IsActive() bool {
	return v.isActive
}

// SetActive sets whether the validator is active
func (v *TestValidator) SetActive(active bool) {
	v.isActive = active
	v.lastUpdate = time.Now()
}

// LastUpdate returns the last update time
func (v *TestValidator) LastUpdate() time.Time {
	return v.lastUpdate
}

// Addr returns the validator address
func (v *TestValidator) Addr() string {
	return v.server.Addr()
}

// MetricsURL returns the validator metrics URL
func (v *TestValidator) MetricsURL() string {
	return v.metrics.URL()
}

// handleConnection handles incoming TCP connections
func (v *TestValidator) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Simple protocol: read command and respond
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		cmd := string(buf[:n])
		switch cmd {
		case "ping":
			conn.Write([]byte("pong"))
		case "status":
			status := fmt.Sprintf("active=%v stake=%f last_update=%d",
				v.isActive, v.stake, v.lastUpdate.Unix())
			conn.Write([]byte(status))
		default:
			conn.Write([]byte(fmt.Sprintf("unknown command: %s", cmd)))
		}
	}
}

// Connect creates a connection to the validator
func (v *TestValidator) Connect() (net.Conn, error) {
	return v.server.Connect()
}

// RequireConnected asserts that a connection can be established
func (v *TestValidator) RequireConnected(t *testing.T) {
	conn, err := v.Connect()
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "pong", string(buf[:n]))
}

// RequireStatus asserts the validator status
func (v *TestValidator) RequireStatus(t *testing.T, active bool, stake float64) {
	conn, err := v.Connect()
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("status"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	require.NoError(t, err)

	status := string(buf[:n])
	require.Contains(t, status, fmt.Sprintf("active=%v", active))
	require.Contains(t, status, fmt.Sprintf("stake=%f", stake))
}

// WithTestValidator runs a test with a validator
func WithTestValidator(t *testing.T, id string, stake float64, fn func(*TestValidator)) {
	validator := NewTestValidator(t, id, stake)
	fn(validator)
}

// TestValidatorPool represents a pool of test validators
type TestValidatorPool struct {
	t          *testing.T
	validators map[string]*TestValidator
}

// NewTestValidatorPool creates a new test validator pool
func NewTestValidatorPool(t *testing.T) *TestValidatorPool {
	return &TestValidatorPool{
		t:          t,
		validators: make(map[string]*TestValidator),
	}
}

// AddValidator adds a validator to the pool
func (p *TestValidatorPool) AddValidator(id string, stake float64) *TestValidator {
	validator := NewTestValidator(p.t, id, stake)
	p.validators[id] = validator
	return validator
}

// GetValidator returns a validator from the pool
func (p *TestValidatorPool) GetValidator(id string) *TestValidator {
	return p.validators[id]
}

// RemoveValidator removes a validator from the pool
func (p *TestValidatorPool) RemoveValidator(id string) {
	delete(p.validators, id)
}

// Validators returns all validators in the pool
func (p *TestValidatorPool) Validators() []*TestValidator {
	validators := make([]*TestValidator, 0, len(p.validators))
	for _, v := range p.validators {
		validators = append(validators, v)
	}
	return validators
}

// WithTestValidatorPool runs a test with a validator pool
func WithTestValidatorPool(t *testing.T, fn func(*TestValidatorPool)) {
	pool := NewTestValidatorPool(t)
	fn(pool)
}
