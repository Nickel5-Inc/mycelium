/*
Package testutil provides a comprehensive testing framework for the Mycelium project.
It includes utilities for test setup, cleanup, assertions, mocking, fixtures, and more.

# Core Testing Utilities

The framework provides several core testing utilities:

  - Cleanup: Manages test resource cleanup
  - Context: Handles test context management with timeouts and cancellation
  - Assertions: Provides extended test assertions
  - Logging: Captures and verifies log output
  - Mocking: Creates and manages test mocks
  - Fixtures: Loads and manages test data
  - Random: Generates random test data

Example Usage:

Basic test with cleanup:

	func TestExample(t *testing.T) {
	    cleanup := NewTestCleanup(t)
	    defer cleanup.Run()

	    dir := TestDir(t)
	    cleanup.AddDir(dir)

	    // Test implementation
	}

Test with context timeout:

	func TestWithContext(t *testing.T) {
	    ctx := NewTestContextWithTimeout(t, 5*time.Second)
	    defer ctx.Cancel()

	    // Test implementation using ctx.Context()
	}

Test with assertions:

	func TestWithAssertions(t *testing.T) {
	    RequireEventually(t, func() bool {
	        // Check condition
	        return true
	    }, 5*time.Second)

	    RequireDeepEqual(t, expected, actual)
	}

Test with logging:

	func TestWithLogging(t *testing.T) {
	    logger := NewTestLogger(t)

	    // Perform operations that generate logs

	    logger.RequireContains(t, "expected log message")
	}

Test with mocking:

	func TestWithMock(t *testing.T) {
	    mock := NewMockObject(t)

	    mock.On("method").Return("result")

	    result := mock.Call("method")
	    // Assert result
	}

Test with fixtures:

	func TestWithFixture(t *testing.T) {
	    fixture := NewTestFixture(t)

	    fixture.Load("testdata/fixture.json")
	    value := fixture.Get("key")
	    // Use fixture data
	}

Test with random data:

	func TestWithRandomData(t *testing.T) {
	    str := RandomString(10)
	    num := RandomInt(1, 100)
	    time := RandomTime(start, end)
	    // Use random test data
	}

# Best Practices

1. Always use cleanup to manage test resources:
  - Add directories and files to cleanup
  - Register cleanup functions
  - Use defer to ensure cleanup runs

2. Use contexts for timeouts and cancellation:
  - Set appropriate timeouts for async operations
  - Cancel contexts when done
  - Check for context cancellation

3. Use assertions effectively:
  - Choose the right assertion for the check
  - Provide helpful error messages
  - Use timeouts for eventual consistency

4. Structure test fixtures well:
  - Keep fixtures in testdata directory
  - Use clear naming conventions
  - Document fixture format and purpose

5. Write effective mocks:
  - Mock only what's necessary
  - Verify mock expectations
  - Clean up mock state between tests

6. Handle test logging:
  - Capture logs during tests
  - Verify log contents
  - Clear log buffer between tests

7. Generate good test data:
  - Use appropriate random generators
  - Set reasonable bounds
  - Document data requirements

# Error Handling

The framework provides several ways to handle errors:

1. RequireNoError: Fails test if error is not nil
2. RequireError: Fails test if error doesn't match expected
3. RequireErrorContains: Fails test if error doesn't contain string
4. RequireErrorIs: Fails test if error doesn't match target
5. RequireErrorAs: Fails test if error can't be type asserted

# Timeouts and Retries

For handling eventual consistency:

1. RequireEventually: Asserts condition becomes true
2. RequireNever: Asserts condition never becomes true
3. WaitForCondition: Waits for condition with timeout
4. RetryWithBackoff: Retries operation with exponential backoff

# Contributing

When adding to the framework:

1. Follow existing patterns
2. Add comprehensive tests
3. Document thoroughly
4. Use builder pattern for complex configs
5. Keep utilities focused and composable
*/
package testutil
