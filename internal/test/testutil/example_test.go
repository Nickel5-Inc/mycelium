package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"mycelium/internal/test/testutil"
)

// ExampleTestCleanup demonstrates using test cleanup
func ExampleTestCleanup() {
	t := &testing.T{}
	testutil.WithTestCleanup(t, func(cleanup *testutil.TestCleanup) {
		// Create temporary directory
		dir := testutil.CreateTestDir(t)
		cleanup.AddDir(dir)

		// Add cleanup function
		cleanup.AddCleanup(func() error {
			// Cleanup resources
			return nil
		})
	})
}

// ExampleTestFixture demonstrates using test fixtures
func ExampleTestFixture() {
	t := &testing.T{}
	testutil.WithTestFixture(t, func(fixture *testutil.TestFixture) {
		// Set fixture data
		fixture.Set("string", "test")
		fixture.Set("number", 42)
		fixture.Set("bool", true)

		// Save fixture to file
		err := fixture.Save("fixture.json")
		require.NoError(t, err)
	})
}

// ExampleTestMock demonstrates using test mocks
func ExampleTestMock() {
	t := &testing.T{}
	testutil.WithMockObject(t, func(mock *testutil.MockObject) {
		// Set up mock function
		fn := mock.On("test")
		fn.SetHandler(func(args ...interface{}) interface{} {
			return "mocked"
		})

		// Call mock function
		result := mock.Call("test")
		require.Equal(t, "mocked", result)
	})
}

// ExampleTestContext demonstrates using test contexts
func ExampleTestContext() {
	t := &testing.T{}
	testutil.WithTestContext(t, func(ctx *testutil.TestContext) {
		// Get context with timeout
		timeoutCtx, cancel := context.WithTimeout(ctx.Context(), 5*time.Second)
		defer cancel()

		// Assert not done
		ctx.RequireNotDone(t)

		// Use context
		select {
		case <-timeoutCtx.Done():
			t.Error("Context timed out")
		default:
			// Context still valid
		}
	})
}

// ExampleTestMetrics demonstrates using test metrics
func ExampleTestMetrics() {
	t := &testing.T{}
	testutil.WithTestMetrics(t, func(metrics *testutil.TestMetrics) {
		// Create and register counter
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_counter",
			Help: "Test counter",
		})
		metrics.Register("test_counter", counter)

		// Increment counter
		counter.Inc()

		// Assert metric value
		metrics.RequireMetricEquals("test_counter", 1)
	})
}

// ExampleTestLogger demonstrates using test logging
func ExampleTestLogger() {
	t := &testing.T{}
	testutil.WithTestLogger(t, func(logger *testutil.TestLogger) {
		// Log messages
		logger.Logger().Info("test message")

		// Assert log contents
		logger.RequireContains(t, "test message")
	})
}

// ExampleTestValidator demonstrates using test validators
func ExampleTestValidator() {
	t := &testing.T{}
	testutil.WithTestValidator(t, "test_validator", 1000, func(validator *testutil.TestValidator) {
		// Get validator info
		validator.RequireStatus(t, true, 1000)

		// Update validator
		validator.SetStake(2000)
		validator.RequireStatus(t, true, 2000)
	})
}

// ExampleTestChain demonstrates using test chain
func ExampleTestChain() {
	t := &testing.T{}
	testutil.WithTestChain(t, func(chain *testutil.TestChain) {
		// Add block
		hash := chain.AddBlock(types.Block{
			Header: types.Header{
				Number: types.BlockNumber(1),
			},
		})

		// Assert block exists
		chain.RequireBlockExists(t, hash)
		chain.RequireHeight(t, 1)
	})
}

// ExampleBenchmarkSuite demonstrates using benchmark suite
func ExampleBenchmarkSuite() {
	b := &testing.B{}
	config := testutil.DefaultBenchmarkConfig()
	testutil.WithBenchmarkSuite(b, config, func(suite *testutil.BenchmarkSuite) {
		// Run parallel benchmark
		suite.RunParallel("test_parallel", func(pb *testing.PB) {
			for pb.Next() {
				// Benchmark operation
			}
		})

		// Run iterative benchmark
		suite.RunIterative("test_iterative", func(i int) {
			// Benchmark operation
		})
	})
}

// ExampleRetryBuilder demonstrates using retry builder
func ExampleRetryBuilder() {
	t := &testing.T{}
	builder := testutil.NewRetryBuilder(t).
		WithAttempts(3).
		WithDelay(time.Second).
		WithTimeout(5 * time.Second)

	// Retry operation
	err := builder.Retry(func() error {
		// Operation that might fail
		return nil
	})
	require.NoError(t, err)
}

// ExampleCompareBuilder demonstrates using compare builder
func ExampleCompareBuilder() {
	t := &testing.T{}
	builder := testutil.NewCompareBuilder(t).
		WithFloatTolerance(0.001).
		WithTimeTolerance(time.Second).
		WithIgnoreFields("id", "timestamp")

	// Compare values
	expected := struct {
		Name  string
		Value float64
	}{
		Name:  "test",
		Value: 1.0,
	}
	actual := struct {
		Name  string
		Value float64
	}{
		Name:  "test",
		Value: 1.001,
	}
	builder.Compare(expected, actual)
}

// ExampleRandomBuilder demonstrates using random data generation
func ExampleRandomBuilder() {
	// Generate random values
	str := testutil.RandomString(10)
	num := testutil.RandomInt(1, 100)
	t := testutil.RandomTime(time.Now(), time.Now().Add(24*time.Hour))

	// Use random values
	_ = str
	_ = num
	_ = t
}
