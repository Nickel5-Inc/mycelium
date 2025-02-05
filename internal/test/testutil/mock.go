package testutil

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MockCall represents a recorded function call
type MockCall struct {
	Name string
	Args []interface{}
}

// MockCalls is a slice of MockCall
type MockCalls []MockCall

// MockRecorder records and verifies mock calls
type MockRecorder struct {
	t      *testing.T
	mu     sync.RWMutex
	calls  MockCalls
	waiter *sync.Cond
}

// NewMockRecorder creates a new mock recorder
func NewMockRecorder(t *testing.T) *MockRecorder {
	r := &MockRecorder{
		t:     t,
		calls: make(MockCalls, 0),
	}
	r.waiter = sync.NewCond(&r.mu)
	return r
}

// Record records a function call
func (r *MockRecorder) Record(name string, args ...interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, MockCall{
		Name: name,
		Args: args,
	})
	r.waiter.Broadcast()
}

// Calls returns all recorded calls
func (r *MockRecorder) Calls() MockCalls {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.calls
}

// Clear clears all recorded calls
func (r *MockRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = make(MockCalls, 0)
}

// matchArgs checks if call arguments match expected arguments
func matchArgs(actual, expected []interface{}) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i, arg := range actual {
		if !reflect.DeepEqual(arg, expected[i]) {
			return false
		}
	}
	return true
}

// WaitForCall waits for a specific call to be recorded
func (r *MockRecorder) WaitForCall(timeout time.Duration, name string, args ...interface{}) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, call := range r.calls {
			if call.Name == name && matchArgs(call.Args, args) {
				r.mu.Unlock()
				return nil
			}
		}
		r.waiter.Wait()
		r.mu.Unlock()
	}
	return context.DeadlineExceeded
}

// RequireCall asserts that a specific call was recorded
func (r *MockRecorder) RequireCall(t *testing.T, name string, args ...interface{}) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, call := range r.calls {
		if call.Name == name && matchArgs(call.Args, args) {
			return
		}
	}
	t.Errorf("Expected call not found: %s", name)
}

// RequireNoCall asserts that a specific call was not recorded
func (r *MockRecorder) RequireNoCall(t *testing.T, name string, args ...interface{}) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, call := range r.calls {
		if call.Name == name && matchArgs(call.Args, args) {
			t.Errorf("Unexpected call found: %s", name)
			return
		}
	}
}

// RequireCallCount asserts the number of times a call was recorded
func (r *MockRecorder) RequireCallCount(t *testing.T, name string, count int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	actual := 0
	for _, call := range r.calls {
		if call.Name == name {
			actual++
		}
	}
	require.Equal(t, count, actual, "Wrong number of calls for %s", name)
}

// MockFunction represents a mock function
type MockFunction struct {
	t        *testing.T
	name     string
	recorder *MockRecorder
	handler  func(args ...interface{}) interface{}
}

// NewMockFunction creates a new mock function
func NewMockFunction(t *testing.T, name string, recorder *MockRecorder) *MockFunction {
	return &MockFunction{
		t:        t,
		name:     name,
		recorder: recorder,
	}
}

// Call executes the mock function
func (f *MockFunction) Call(args ...interface{}) interface{} {
	f.recorder.Record(f.name, args...)
	if f.handler != nil {
		return f.handler(args...)
	}
	return nil
}

// SetHandler sets the function handler
func (f *MockFunction) SetHandler(handler func(args ...interface{}) interface{}) {
	f.handler = handler
}

// MockObject represents a mock object
type MockObject struct {
	t        *testing.T
	recorder *MockRecorder
	funcs    map[string]*MockFunction
}

// NewMockObject creates a new mock object
func NewMockObject(t *testing.T) *MockObject {
	return &MockObject{
		t:        t,
		recorder: NewMockRecorder(t),
		funcs:    make(map[string]*MockFunction),
	}
}

// On creates or returns a mock function
func (o *MockObject) On(name string) *MockFunction {
	if fn, ok := o.funcs[name]; ok {
		return fn
	}
	fn := NewMockFunction(o.t, name, o.recorder)
	o.funcs[name] = fn
	return fn
}

// Call calls a mock function
func (o *MockObject) Call(name string, args ...interface{}) interface{} {
	fn, ok := o.funcs[name]
	if !ok {
		o.t.Errorf("Mock function not found: %s", name)
		return nil
	}
	return fn.Call(args...)
}

// Recorder returns the mock recorder
func (o *MockObject) Recorder() *MockRecorder {
	return o.recorder
}

// WithMockObject runs a test with a mock object
func WithMockObject(t *testing.T, fn func(*MockObject)) {
	mock := NewMockObject(t)
	fn(mock)
}

// MockContextKey is a context key for mock objects
type MockContextKey struct {
	name string
}

// String returns the string representation of the key
func (k MockContextKey) String() string {
	return k.name
}

// WithMockContext runs a test with a mock object in context
func WithMockContext(ctx context.Context, t *testing.T, fn func(context.Context, *MockObject)) {
	mock := NewMockObject(t)
	ctx = context.WithValue(ctx, MockContextKey{"mock"}, mock)
	fn(ctx, mock)
}

// GetMockObject gets a mock object from context
func GetMockObject(ctx context.Context) *MockObject {
	if mock, ok := ctx.Value(MockContextKey{"mock"}).(*MockObject); ok {
		return mock
	}
	return nil
}
