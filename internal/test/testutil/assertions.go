package testutil

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// RequireEventually asserts that a condition becomes true within a timeout
func RequireEventually(t *testing.T, condition func() bool, timeout time.Duration, msgAndArgs ...interface{}) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Fail(t, "Condition not met within timeout", msgAndArgs...)
}

// RequireNever asserts that a condition never becomes true within a timeout
func RequireNever(t *testing.T, condition func() bool, timeout time.Duration, msgAndArgs ...interface{}) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			require.Fail(t, "Condition unexpectedly met", msgAndArgs...)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// RequireDeepEqual asserts deep equality between expected and actual values
func RequireDeepEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	if !reflect.DeepEqual(expected, actual) {
		require.Fail(t, fmt.Sprintf("Not equal: \nexpected: %v\nactual  : %v", expected, actual), msgAndArgs...)
	}
}

// RequireNotDeepEqual asserts deep inequality between expected and actual values
func RequireNotDeepEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	if reflect.DeepEqual(expected, actual) {
		require.Fail(t, fmt.Sprintf("Should not be equal: %v", expected), msgAndArgs...)
	}
}

// RequireZero asserts that a value is zero
func RequireZero(t *testing.T, actual interface{}, msgAndArgs ...interface{}) {
	if !reflect.ValueOf(actual).IsZero() {
		require.Fail(t, fmt.Sprintf("Should be zero value, got: %v", actual), msgAndArgs...)
	}
}

// RequireNotZero asserts that a value is not zero
func RequireNotZero(t *testing.T, actual interface{}, msgAndArgs ...interface{}) {
	if reflect.ValueOf(actual).IsZero() {
		require.Fail(t, fmt.Sprintf("Should not be zero value: %v", actual), msgAndArgs...)
	}
}

// RequireEmpty asserts that a value is empty
func RequireEmpty(t *testing.T, actual interface{}, msgAndArgs ...interface{}) {
	v := reflect.ValueOf(actual)
	if v.Len() != 0 {
		require.Fail(t, fmt.Sprintf("Should be empty, got length: %d", v.Len()), msgAndArgs...)
	}
}

// RequireNotEmpty asserts that a value is not empty
func RequireNotEmpty(t *testing.T, actual interface{}, msgAndArgs ...interface{}) {
	v := reflect.ValueOf(actual)
	if v.Len() == 0 {
		require.Fail(t, "Should not be empty", msgAndArgs...)
	}
}

// RequireContainsAll asserts that a value contains all elements
func RequireContainsAll(t *testing.T, actual interface{}, elements ...interface{}) {
	v := reflect.ValueOf(actual)
	for _, element := range elements {
		found := false
		for i := 0; i < v.Len(); i++ {
			if reflect.DeepEqual(v.Index(i).Interface(), element) {
				found = true
				break
			}
		}
		if !found {
			require.Fail(t, fmt.Sprintf("Missing element: %v", element))
		}
	}
}

// RequireContainsNone asserts that a value contains none of the elements
func RequireContainsNone(t *testing.T, actual interface{}, elements ...interface{}) {
	v := reflect.ValueOf(actual)
	for _, element := range elements {
		for i := 0; i < v.Len(); i++ {
			if reflect.DeepEqual(v.Index(i).Interface(), element) {
				require.Fail(t, fmt.Sprintf("Found unexpected element: %v", element))
			}
		}
	}
}

// RequireContainsExactly asserts that a value contains exactly the given elements
func RequireContainsExactly(t *testing.T, actual interface{}, elements ...interface{}) {
	v := reflect.ValueOf(actual)
	if v.Len() != len(elements) {
		require.Fail(t, fmt.Sprintf("Wrong length: got %d, want %d", v.Len(), len(elements)))
		return
	}

	actualSlice := make([]interface{}, v.Len())
	for i := 0; i < v.Len(); i++ {
		actualSlice[i] = v.Index(i).Interface()
	}

	sort.Slice(actualSlice, func(i, j int) bool {
		return fmt.Sprintf("%v", actualSlice[i]) < fmt.Sprintf("%v", actualSlice[j])
	})
	sort.Slice(elements, func(i, j int) bool {
		return fmt.Sprintf("%v", elements[i]) < fmt.Sprintf("%v", elements[j])
	})

	if !reflect.DeepEqual(actualSlice, elements) {
		require.Fail(t, fmt.Sprintf("Not equal: \nexpected: %v\nactual  : %v", elements, actualSlice))
	}
}

// RequireOrdered asserts that a value is ordered according to less function
func RequireOrdered(t *testing.T, actual interface{}, less func(i, j int) bool) {
	v := reflect.ValueOf(actual)
	for i := 1; i < v.Len(); i++ {
		if !less(i-1, i) {
			require.Fail(t, fmt.Sprintf("Not ordered at index %d", i))
		}
	}
}

// RequireUnique asserts that all elements in a value are unique
func RequireUnique(t *testing.T, actual interface{}) {
	v := reflect.ValueOf(actual)
	seen := make(map[interface{}]bool)
	for i := 0; i < v.Len(); i++ {
		element := v.Index(i).Interface()
		if seen[element] {
			require.Fail(t, fmt.Sprintf("Duplicate element found: %v", element))
		}
		seen[element] = true
	}
}

// RequireImplements asserts that a value implements an interface
func RequireImplements(t *testing.T, interfaceType interface{}, value interface{}) {
	valueType := reflect.TypeOf(value)
	if !valueType.Implements(reflect.TypeOf(interfaceType).Elem()) {
		require.Fail(t, fmt.Sprintf("%v does not implement %v", valueType, reflect.TypeOf(interfaceType).Elem()))
	}
}

// RequireType asserts that a value is of a specific type
func RequireType(t *testing.T, expectedType interface{}, value interface{}) {
	valueType := reflect.TypeOf(value)
	expectedType = reflect.TypeOf(expectedType)
	if valueType != expectedType {
		require.Fail(t, fmt.Sprintf("Wrong type: got %v, want %v", valueType, expectedType))
	}
}

// RequireAssignable asserts that a value is assignable to a type
func RequireAssignable(t *testing.T, targetType interface{}, value interface{}) {
	valueType := reflect.TypeOf(value)
	targetTypeReflect := reflect.TypeOf(targetType)
	if !valueType.AssignableTo(targetTypeReflect) {
		require.Fail(t, fmt.Sprintf("%v is not assignable to %v", valueType, targetTypeReflect))
	}
}

// RequireConvertible asserts that a value is convertible to a type
func RequireConvertible(t *testing.T, targetType interface{}, value interface{}) {
	valueType := reflect.TypeOf(value)
	targetTypeReflect := reflect.TypeOf(targetType)
	if !valueType.ConvertibleTo(targetTypeReflect) {
		require.Fail(t, fmt.Sprintf("%v is not convertible to %v", valueType, targetTypeReflect))
	}
}
