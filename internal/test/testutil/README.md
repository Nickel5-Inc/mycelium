# Mycelium Test Framework

The Mycelium test framework provides a comprehensive set of utilities for testing the Mycelium project. It includes tools for test setup, cleanup, assertions, mocking, fixtures, networking, storage, metrics, logging, and more.

## Features

### Test Setup and Cleanup

- `TestDir`: Creates temporary test directories
- `TestCleanup`: Manages test cleanup operations
- `WithTestCleanup`: Runs tests with automatic cleanup

```go
WithTestCleanup(t, func(cleanup *TestCleanup) {
    dir := TestDir(t)
    cleanup.AddDir(dir)
    cleanup.AddCleanup(func() error {
        // Cleanup implementation
        return nil
    })
})
```

### Test Assertions

- `Compare`: Deep comparison with configurable options
- `RequireEventually`: Asserts conditions with timeout
- `RequireRetry`: Retries assertions with backoff

```go
builder := NewCompareBuilder(t).
    WithFloatTolerance(1e-9).
    WithTimeTolerance(time.Millisecond).
    WithIgnoreFields("id", "timestamp")

builder.Compare(expected, actual)
```

### Test Data Generation

- `RandomString`: Generates random strings
- `RandomBytes`: Generates random byte slices
- `RandomInt`: Generates random integers
- `RandomTime`: Generates random timestamps

```go
str := RandomString(10)
bytes := RandomBytes(20)
num := RandomInt(1, 100)
timestamp := RandomTime(start, end)
```

### Test Fixtures

- `TestFixture`: Manages test data fixtures
- `WithTestFixture`: Runs tests with fixtures
- `TestFixtureBuilder`: Helps build test fixtures

```go
WithTestFixture(t, func(fixture *TestFixture) {
    fixture.Set("key", "value")
    fixture.Save("fixture.json")
    fixture.Load("fixture.json")
})
```

### Test Mocking

- `MockObject`: Generic mock object implementation
- `MockRecorder`: Records mock function calls
- `MockFunction`: Configurable mock functions

```go
WithMockObject(t, func(mock *MockObject) {
    fn := mock.On("method")
    fn.SetHandler(func(args ...interface{}) interface{} {
        return "result"
    })
    result := mock.Call("method", "arg")
})
```

### Test Networking

- `TestHTTPServer`: HTTP server for testing
- `TestGRPCServer`: gRPC server for testing
- `TestTCPServer`: TCP server for testing

```go
WithTestHTTPServer(t, handler, func(server *TestHTTPServer) {
    resp, err := http.Get(server.URL())
    // Test implementation
})
```

### Test Storage

- `TestStorage`: Manages test storage operations
- `WithTestStorage`: Runs tests with storage
- `TestStorageBuilder`: Helps build test storage

```go
WithTestStorage(t, func(storage *TestStorage) {
    storage.Write("key", []byte("value"))
    data, err := storage.Read("key")
    // Test implementation
})
```

### Test Metrics

- `TestMetrics`: Manages test metrics collection
- `WithTestMetrics`: Runs tests with metrics
- `TestMetricsBuilder`: Helps build test metrics

```go
WithTestMetrics(t, func(metrics *TestMetrics) {
    metrics.Register("counter", prometheus.NewCounter())
    metrics.RequireMetricEquals("counter", 1)
})
```

### Test Logging

- `TestLogger`: Manages test logging
- `WithTestLogger`: Runs tests with logging
- `TestLogHook`: Captures log entries

```go
WithTestLogger(t, func(logger *TestLogger) {
    log := logger.Logger()
    log.Info("message")
    logger.RequireContains(t, "message")
})
```

### Test Context

- `TestContext`: Manages test contexts
- `WithTestContext`: Runs tests with context
- `TestContextBuilder`: Helps build test contexts

```go
WithTestContext(t, func(ctx *TestContext) {
    context := ctx.Context()
    ctx.Cancel()
    ctx.RequireDone(t)
})
```

### Test Benchmarking

- `BenchmarkRunner`: Runs benchmarks
- `BenchmarkSuite`: Manages benchmark suites
- `BenchmarkConfig`: Configures benchmarks

```go
WithBenchmarkSuite(b, config, func(suite *BenchmarkSuite) {
    suite.RunParallel("test", func(pb *testing.PB) {
        for pb.Next() {
            // Benchmark implementation
        }
    })
})
```

### Test Retry

- `RetryConfig`: Configures retry behavior
- `Retry`: Retries operations with backoff
- `RetryBuilder`: Helps build retry config

```go
builder := NewRetryBuilder(t).
    WithAttempts(3).
    WithDelay(100 * time.Millisecond).
    WithTimeout(5 * time.Second)

err := builder.Retry(func() error {
    // Operation implementation
    return nil
})
```

## Usage

1. Import the package:

```go
import "mycelium/internal/test/testutil"
```

2. Use the utilities in your tests:

```go
func TestExample(t *testing.T) {
    WithTestCleanup(t, func(cleanup *TestCleanup) {
        // Create temporary directory
        dir := TestDir(t)
        cleanup.AddDir(dir)

        // Create test fixture
        WithTestFixture(t, func(fixture *TestFixture) {
            fixture.Set("key", "value")

            // Create mock object
            WithMockObject(t, func(mock *MockObject) {
                mock.On("method").Return("result")

                // Run test with context
                WithTestContext(t, func(ctx *TestContext) {
                    // Test implementation
                })
            })
        })
    })
}
```

3. Use the utilities in your benchmarks:

```go
func BenchmarkExample(b *testing.B) {
    WithBenchmarkSuite(b, DefaultBenchmarkConfig(), func(suite *BenchmarkSuite) {
        suite.RunParallel("test", func(pb *testing.PB) {
            for pb.Next() {
                // Benchmark implementation
            }
        })
    })
}
```

## Best Practices

1. Use the `With*` functions to ensure proper cleanup and resource management.
2. Use builders to configure complex test utilities.
3. Use the retry utilities for flaky tests or operations.
4. Use the comparison utilities for deep equality checks.
5. Use the random utilities for generating test data.
6. Use the mock utilities for isolating dependencies.
7. Use the metrics utilities for testing instrumentation.
8. Use the logging utilities for testing log output.
9. Use the context utilities for testing timeouts and cancellation.
10. Use the benchmark utilities for performance testing.

## Contributing

1. Follow the existing patterns and conventions.
2. Add tests for new functionality.
3. Update documentation for significant changes.
4. Use the builders pattern for complex configurations.
5. Keep the utilities focused and composable. 