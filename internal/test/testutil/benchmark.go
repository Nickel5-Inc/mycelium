package testutil

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// BenchmarkResult represents a benchmark result
type BenchmarkResult struct {
	Name     string
	Duration time.Duration
	Ops      int64
	BytesOp  int64
	AllocsOp int64
}

// String returns a string representation of the result
func (r BenchmarkResult) String() string {
	return fmt.Sprintf(
		"%s: %s, %d ops, %d B/op, %d allocs/op",
		r.Name,
		r.Duration,
		r.Ops,
		r.BytesOp,
		r.AllocsOp,
	)
}

// BenchmarkRunner runs benchmarks
type BenchmarkRunner struct {
	b       *testing.B
	results []BenchmarkResult
}

// NewBenchmarkRunner creates a new benchmark runner
func NewBenchmarkRunner(b *testing.B) *BenchmarkRunner {
	return &BenchmarkRunner{
		b:       b,
		results: make([]BenchmarkResult, 0),
	}
}

// Run runs a benchmark
func (r *BenchmarkRunner) Run(name string, fn func(b *testing.B)) {
	r.b.Run(name, func(b *testing.B) {
		b.ReportAllocs()
		start := time.Now()
		fn(b)
		duration := time.Since(start)

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		r.results = append(r.results, BenchmarkResult{
			Name:     name,
			Duration: duration,
			Ops:      int64(b.N),
			BytesOp:  int64(memStats.TotalAlloc) / int64(b.N),
			AllocsOp: int64(memStats.Mallocs) / int64(b.N),
		})
	})
}

// Results returns all benchmark results
func (r *BenchmarkRunner) Results() []BenchmarkResult {
	return append([]BenchmarkResult{}, r.results...)
}

// BenchmarkConfig represents benchmark configuration
type BenchmarkConfig struct {
	Iterations   int
	Parallel     int
	WarmupTime   time.Duration
	CooldownTime time.Duration
}

// DefaultBenchmarkConfig returns default benchmark configuration
func DefaultBenchmarkConfig() BenchmarkConfig {
	return BenchmarkConfig{
		Iterations:   5,
		Parallel:     runtime.GOMAXPROCS(0),
		WarmupTime:   time.Second,
		CooldownTime: time.Second,
	}
}

// RunParallelBenchmark runs a parallel benchmark
func RunParallelBenchmark(b *testing.B, config BenchmarkConfig, fn func(pb *testing.PB)) {
	// Warmup
	time.Sleep(config.WarmupTime)

	b.SetParallelism(config.Parallel)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		fn(pb)
	})

	b.StopTimer()

	// Cooldown
	time.Sleep(config.CooldownTime)
}

// RunIterativeBenchmark runs an iterative benchmark
func RunIterativeBenchmark(b *testing.B, config BenchmarkConfig, fn func(iteration int)) {
	// Warmup
	time.Sleep(config.WarmupTime)

	b.ResetTimer()

	for i := 0; i < config.Iterations; i++ {
		fn(i)
	}

	b.StopTimer()

	// Cooldown
	time.Sleep(config.CooldownTime)
}

// BenchmarkSuite represents a benchmark suite
type BenchmarkSuite struct {
	runner *BenchmarkRunner
	config BenchmarkConfig
}

// NewBenchmarkSuite creates a new benchmark suite
func NewBenchmarkSuite(b *testing.B, config BenchmarkConfig) *BenchmarkSuite {
	return &BenchmarkSuite{
		runner: NewBenchmarkRunner(b),
		config: config,
	}
}

// RunParallel runs a parallel benchmark
func (s *BenchmarkSuite) RunParallel(name string, fn func(pb *testing.PB)) {
	s.runner.Run(name, func(b *testing.B) {
		RunParallelBenchmark(b, s.config, fn)
	})
}

// RunIterative runs an iterative benchmark
func (s *BenchmarkSuite) RunIterative(name string, fn func(iteration int)) {
	s.runner.Run(name, func(b *testing.B) {
		RunIterativeBenchmark(b, s.config, fn)
	})
}

// Results returns all benchmark results
func (s *BenchmarkSuite) Results() []BenchmarkResult {
	return s.runner.Results()
}

// WithBenchmarkSuite runs a benchmark suite
func WithBenchmarkSuite(b *testing.B, config BenchmarkConfig, fn func(*BenchmarkSuite)) {
	suite := NewBenchmarkSuite(b, config)
	fn(suite)
}

// BenchmarkMemory measures memory usage
func BenchmarkMemory(b *testing.B, fn func()) (uint64, uint64) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	allocsBefore := memStats.TotalAlloc
	mallocsBefore := memStats.Mallocs

	fn()

	runtime.ReadMemStats(&memStats)
	allocsAfter := memStats.TotalAlloc
	mallocsAfter := memStats.Mallocs

	return allocsAfter - allocsBefore, mallocsAfter - mallocsBefore
}

// BenchmarkTime measures execution time
func BenchmarkTime(fn func()) time.Duration {
	start := time.Now()
	fn()
	return time.Since(start)
}

// BenchmarkCPU measures CPU usage
func BenchmarkCPU(fn func()) float64 {
	var cpuStats runtime.MemStats
	runtime.ReadMemStats(&cpuStats)
	startTime := time.Now()
	startCPU := cpuStats.GCCPUFraction

	fn()

	runtime.ReadMemStats(&cpuStats)
	endTime := time.Now()
	endCPU := cpuStats.GCCPUFraction

	duration := endTime.Sub(startTime).Seconds()
	cpuTime := endCPU - startCPU

	return cpuTime / duration
}
