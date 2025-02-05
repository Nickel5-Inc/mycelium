package testutil

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
)

// TestMetrics represents a test metrics server
type TestMetrics struct {
	t          *testing.T
	server     *TestHTTPServer
	registry   *prometheus.Registry
	collectors map[string]prometheus.Collector
	mu         sync.RWMutex
}

// NewTestMetrics creates a new test metrics server
func NewTestMetrics(t *testing.T) *TestMetrics {
	registry := prometheus.NewRegistry()
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	server := NewTestHTTPServer(t, handler)

	return &TestMetrics{
		t:          t,
		server:     server,
		registry:   registry,
		collectors: make(map[string]prometheus.Collector),
	}
}

// Register registers a collector with the metrics registry
func (m *TestMetrics) Register(name string, collector prometheus.Collector) {
	require.NoError(m.t, m.registry.Register(collector))
	m.collectors[name] = collector
}

// URL returns the metrics server URL
func (m *TestMetrics) URL() string {
	return m.server.URL()
}

// GetMetric returns the current value of a metric
func (m *TestMetrics) GetMetric(name string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	collector, ok := m.collectors[name]
	if !ok {
		return 0, fmt.Errorf("metric not found: %s", name)
	}

	// Get metric family
	metricChan := make(chan prometheus.Metric, 1)
	collector.(prometheus.Collector).Collect(metricChan)
	close(metricChan)

	// Get the first metric
	metric := <-metricChan

	// Extract value using reflection since we can't access internal fields directly
	metricVal := reflect.ValueOf(metric)
	if metricVal.Kind() == reflect.Ptr {
		metricVal = metricVal.Elem()
	}

	// Look for a Value field
	valueField := metricVal.FieldByName("Value")
	if valueField.IsValid() {
		return valueField.Float(), nil
	}

	return 0, fmt.Errorf("could not extract value from metric")
}

// RequireMetricEquals asserts that a metric equals an expected value
func (m *TestMetrics) RequireMetricEquals(name string, expected float64) {
	value, err := m.GetMetric(name)
	require.NoError(m.t, err)
	require.Equal(m.t, expected, value)
}

// RequireMetricGreaterThan asserts that a metric is greater than an expected value
func (m *TestMetrics) RequireMetricGreaterThan(name string, expected float64) {
	value, err := m.GetMetric(name)
	require.NoError(m.t, err)
	require.Greater(m.t, value, expected)
}

// RequireMetricLessThan asserts that a metric is less than an expected value
func (m *TestMetrics) RequireMetricLessThan(name string, expected float64) {
	value, err := m.GetMetric(name)
	require.NoError(m.t, err)
	require.Less(m.t, value, expected)
}

// WaitForMetric waits for a metric to equal an expected value
func (m *TestMetrics) WaitForMetric(name string, expected float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if value, err := m.GetMetric(name); err == nil && value == expected {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("metric %s did not equal %f within %s", name, expected, timeout)
}

// WaitForMetricGreaterThan waits for a metric to be greater than an expected value
func (m *TestMetrics) WaitForMetricGreaterThan(name string, expected float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if value, err := m.GetMetric(name); err == nil && value > expected {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("metric %s did not exceed %f within %s", name, expected, timeout)
}

// WaitForMetricLessThan waits for a metric to be less than an expected value
func (m *TestMetrics) WaitForMetricLessThan(name string, expected float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if value, err := m.GetMetric(name); err == nil && value < expected {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("metric %s did not fall below %f within %s", name, expected, timeout)
}

// GetMetricsOutput returns the raw metrics output
func (m *TestMetrics) GetMetricsOutput() (string, error) {
	resp, err := http.Get(m.URL())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// RequireMetricsContain asserts that the metrics output contains expected strings
func (m *TestMetrics) RequireMetricsContain(expected ...string) {
	output, err := m.GetMetricsOutput()
	require.NoError(m.t, err)
	for _, exp := range expected {
		require.Contains(m.t, output, exp)
	}
}

// RequireMetricsNotContain asserts that the metrics output does not contain strings
func (m *TestMetrics) RequireMetricsNotContain(unexpected ...string) {
	output, err := m.GetMetricsOutput()
	require.NoError(m.t, err)
	for _, unexp := range unexpected {
		require.NotContains(m.t, output, unexp)
	}
}

// WithTestMetrics runs a test with a metrics server
func WithTestMetrics(t *testing.T, fn func(*TestMetrics)) {
	metrics := NewTestMetrics(t)
	fn(metrics)
}
