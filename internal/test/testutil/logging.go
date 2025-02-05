package testutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestLogger represents a test logger
type TestLogger struct {
	t      *testing.T
	logger *logrus.Logger
	buffer *bytes.Buffer
	mu     sync.RWMutex
}

// NewTestLogger creates a new test logger
func NewTestLogger(t *testing.T) *TestLogger {
	buffer := &bytes.Buffer{}
	logger := logrus.New()
	logger.SetOutput(io.MultiWriter(buffer, os.Stdout))
	logger.SetFormatter(&logrus.TextFormatter{
		DisableColors:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		FullTimestamp:   true,
	})

	return &TestLogger{
		t:      t,
		logger: logger,
		buffer: buffer,
	}
}

// Logger returns the underlying logger
func (l *TestLogger) Logger() *logrus.Logger {
	return l.logger
}

// Buffer returns the log buffer
func (l *TestLogger) Buffer() *bytes.Buffer {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return bytes.NewBuffer(l.buffer.Bytes())
}

// String returns the log contents as string
func (l *TestLogger) String() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.buffer.String()
}

// Clear clears the log buffer
func (l *TestLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buffer.Reset()
}

// RequireContains asserts that the log contains text
func (l *TestLogger) RequireContains(t *testing.T, text string) {
	require.Contains(t, l.String(), text)
}

// RequireNotContains asserts that the log does not contain text
func (l *TestLogger) RequireNotContains(t *testing.T, text string) {
	require.NotContains(t, l.String(), text)
}

// RequireLevel asserts that a log entry has a level
func (l *TestLogger) RequireLevel(t *testing.T, text string, level logrus.Level) {
	logs := l.String()
	require.Contains(t, logs, text)

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if strings.Contains(line, text) {
			require.Contains(t, line, level.String())
			return
		}
	}
	t.Errorf("Log entry not found: %s", text)
}

// RequireField asserts that a log entry has a field
func (l *TestLogger) RequireField(t *testing.T, text string, field string, value interface{}) {
	logs := l.String()
	require.Contains(t, logs, text)

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if strings.Contains(line, text) {
			fieldStr := fmt.Sprintf("%s=%v", field, value)
			require.Contains(t, line, fieldStr)
			return
		}
	}
	t.Errorf("Log entry not found: %s", text)
}

// WithTestLogger runs a test with a logger
func WithTestLogger(t *testing.T, fn func(*TestLogger)) {
	logger := NewTestLogger(t)
	fn(logger)
}

// TestLogCapture captures logs during a test
type TestLogCapture struct {
	t       *testing.T
	logger  *TestLogger
	origLog *logrus.Logger
}

// NewTestLogCapture creates a new log capture
func NewTestLogCapture(t *testing.T) *TestLogCapture {
	logger := NewTestLogger(t)
	origLog := logrus.StandardLogger()

	// Replace standard logger
	*logrus.StandardLogger() = *logger.Logger()

	capture := &TestLogCapture{
		t:       t,
		logger:  logger,
		origLog: origLog,
	}

	t.Cleanup(func() {
		capture.Restore()
	})

	return capture
}

// Logger returns the test logger
func (c *TestLogCapture) Logger() *TestLogger {
	return c.logger
}

// Restore restores the original logger
func (c *TestLogCapture) Restore() {
	*logrus.StandardLogger() = *c.origLog
}

// WithTestLogCapture runs a test with log capture
func WithTestLogCapture(t *testing.T, fn func(*TestLogCapture)) {
	capture := NewTestLogCapture(t)
	fn(capture)
}

// TestLogHook represents a test log hook
type TestLogHook struct {
	t       *testing.T
	mu      sync.RWMutex
	levels  []logrus.Level
	entries []*logrus.Entry
}

// NewTestLogHook creates a new log hook
func NewTestLogHook(t *testing.T, levels ...logrus.Level) *TestLogHook {
	return &TestLogHook{
		t:       t,
		levels:  levels,
		entries: make([]*logrus.Entry, 0),
	}
}

// Levels returns the hook levels
func (h *TestLogHook) Levels() []logrus.Level {
	return h.levels
}

// Fire implements logrus.Hook
func (h *TestLogHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, entry)
	return nil
}

// Entries returns the captured log entries
func (h *TestLogHook) Entries() []*logrus.Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]*logrus.Entry{}, h.entries...)
}

// Clear clears the captured entries
func (h *TestLogHook) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = make([]*logrus.Entry, 0)
}

// RequireEntry asserts that an entry exists
func (h *TestLogHook) RequireEntry(t *testing.T, level logrus.Level, message string) {
	entries := h.Entries()
	for _, entry := range entries {
		if entry.Level == level && entry.Message == message {
			return
		}
	}
	t.Errorf("Log entry not found: [%s] %s", level, message)
}

// RequireNoEntry asserts that an entry does not exist
func (h *TestLogHook) RequireNoEntry(t *testing.T, level logrus.Level, message string) {
	entries := h.Entries()
	for _, entry := range entries {
		if entry.Level == level && entry.Message == message {
			t.Errorf("Unexpected log entry found: [%s] %s", level, message)
			return
		}
	}
}
