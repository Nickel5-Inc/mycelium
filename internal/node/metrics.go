package node

import (
	"github.com/prometheus/client_golang/prometheus"
)

// MetricConfig holds configuration for a Prometheus metric
type MetricConfig struct {
	Name string
	Help string
}

// Metrics holds all Prometheus metrics for the node
type Metrics struct {
	PeerCount        prometheus.Gauge
	ActiveValidators prometheus.Gauge
	SyncProgress     prometheus.Gauge
	ResponseLatency  prometheus.Histogram
	ServingRate      prometheus.Gauge
	MemoryUsage      prometheus.Gauge
	CPUUsage         prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	m := &Metrics{
		PeerCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mycelium_peer_count",
			Help: "Number of connected peers",
		}),
		ActiveValidators: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mycelium_active_validators",
			Help: "Number of active validators",
		}),
		SyncProgress: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mycelium_sync_progress",
			Help: "Database sync progress percentage",
		}),
		ResponseLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mycelium_response_latency",
			Help:    "Response latency in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		ServingRate: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mycelium_serving_rate",
			Help: "Rate of successful responses",
		}),
		MemoryUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mycelium_memory_usage",
			Help: "Memory usage in bytes",
		}),
		CPUUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mycelium_cpu_usage",
			Help: "CPU usage percentage",
		}),
	}

	// Register all metrics
	prometheus.MustRegister(
		m.PeerCount,
		m.ActiveValidators,
		m.SyncProgress,
		m.ResponseLatency,
		m.ServingRate,
		m.MemoryUsage,
		m.CPUUsage,
	)

	return m
}

// Close unregisters all metrics
func (m *Metrics) Close() {
	prometheus.Unregister(m.PeerCount)
	prometheus.Unregister(m.ActiveValidators)
	prometheus.Unregister(m.SyncProgress)
	prometheus.Unregister(m.ResponseLatency)
	prometheus.Unregister(m.ServingRate)
	prometheus.Unregister(m.MemoryUsage)
	prometheus.Unregister(m.CPUUsage)
}
