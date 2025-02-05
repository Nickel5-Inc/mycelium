package testutil

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// CompareConfig represents comparison configuration
type CompareConfig struct {
	FloatTolerance   float64
	TimeTolerance    time.Duration
	IgnoreFields     []string
	IgnoreZeroValues bool
	IgnoreCase       bool
}

// DefaultCompareConfig returns default comparison configuration
func DefaultCompareConfig() CompareConfig {
	return CompareConfig{
		FloatTolerance:   1e-9,
		TimeTolerance:    time.Millisecond,
		IgnoreFields:     nil,
		IgnoreZeroValues: false,
		IgnoreCase:       false,
	}
}

// Compare compares two values with configuration
func Compare(t *testing.T, expected, actual interface{}, config CompareConfig) {
	diff := compareValues(expected, actual, config)
	require.Empty(t, diff, "Values are not equal:\n%s", diff)
}

// compareValues compares two values and returns differences
func compareValues(expected, actual interface{}, config CompareConfig) string {
	if expected == nil && actual == nil {
		return ""
	}
	if expected == nil || actual == nil {
		return fmt.Sprintf("expected: %v, actual: %v", expected, actual)
	}

	expectedValue := reflect.ValueOf(expected)
	actualValue := reflect.ValueOf(actual)

	if expectedValue.Type() != actualValue.Type() {
		return fmt.Sprintf("type mismatch: expected %v, got %v", expectedValue.Type(), actualValue.Type())
	}

	switch expectedValue.Kind() {
	case reflect.Bool:
		if expectedValue.Bool() != actualValue.Bool() {
			return fmt.Sprintf("expected: %v, actual: %v", expectedValue.Bool(), actualValue.Bool())
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if expectedValue.Int() != actualValue.Int() {
			return fmt.Sprintf("expected: %v, actual: %v", expectedValue.Int(), actualValue.Int())
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if expectedValue.Uint() != actualValue.Uint() {
			return fmt.Sprintf("expected: %v, actual: %v", expectedValue.Uint(), actualValue.Uint())
		}

	case reflect.Float32, reflect.Float64:
		if math.Abs(expectedValue.Float()-actualValue.Float()) > config.FloatTolerance {
			return fmt.Sprintf("expected: %v, actual: %v", expectedValue.Float(), actualValue.Float())
		}

	case reflect.String:
		expectedStr := expectedValue.String()
		actualStr := actualValue.String()
		if config.IgnoreCase {
			expectedStr = strings.ToLower(expectedStr)
			actualStr = strings.ToLower(actualStr)
		}
		if expectedStr != actualStr {
			return fmt.Sprintf("expected: %v, actual: %v", expectedValue.String(), actualValue.String())
		}

	case reflect.Struct:
		if expectedValue.Type() == reflect.TypeOf(time.Time{}) {
			expectedTime := expectedValue.Interface().(time.Time)
			actualTime := actualValue.Interface().(time.Time)
			if math.Abs(float64(expectedTime.Sub(actualTime))) > float64(config.TimeTolerance) {
				return fmt.Sprintf("expected: %v, actual: %v", expectedTime, actualTime)
			}
			return ""
		}

		var diffs []string
		for i := 0; i < expectedValue.NumField(); i++ {
			field := expectedValue.Type().Field(i)
			if contains(config.IgnoreFields, field.Name) {
				continue
			}

			expectedField := expectedValue.Field(i)
			actualField := actualValue.Field(i)

			if config.IgnoreZeroValues && isZero(expectedField) {
				continue
			}

			if diff := compareValues(expectedField.Interface(), actualField.Interface(), config); diff != "" {
				diffs = append(diffs, fmt.Sprintf("%s: %s", field.Name, diff))
			}
		}
		if len(diffs) > 0 {
			return strings.Join(diffs, "\n")
		}

	case reflect.Map:
		if expectedValue.Len() != actualValue.Len() {
			return fmt.Sprintf("map length mismatch: expected %d, got %d", expectedValue.Len(), actualValue.Len())
		}

		var diffs []string
		for _, key := range expectedValue.MapKeys() {
			expectedItem := expectedValue.MapIndex(key)
			actualItem := actualValue.MapIndex(key)

			if !actualItem.IsValid() {
				diffs = append(diffs, fmt.Sprintf("missing key %v", key))
				continue
			}

			if diff := compareValues(expectedItem.Interface(), actualItem.Interface(), config); diff != "" {
				diffs = append(diffs, fmt.Sprintf("%v: %s", key, diff))
			}
		}
		if len(diffs) > 0 {
			return strings.Join(diffs, "\n")
		}

	case reflect.Slice, reflect.Array:
		if expectedValue.Len() != actualValue.Len() {
			return fmt.Sprintf("length mismatch: expected %d, got %d", expectedValue.Len(), actualValue.Len())
		}

		var diffs []string
		for i := 0; i < expectedValue.Len(); i++ {
			if diff := compareValues(expectedValue.Index(i).Interface(), actualValue.Index(i).Interface(), config); diff != "" {
				diffs = append(diffs, fmt.Sprintf("[%d]: %s", i, diff))
			}
		}
		if len(diffs) > 0 {
			return strings.Join(diffs, "\n")
		}

	case reflect.Ptr:
		if expectedValue.IsNil() != actualValue.IsNil() {
			return fmt.Sprintf("nil mismatch: expected %v, got %v", expectedValue.IsNil(), actualValue.IsNil())
		}
		if !expectedValue.IsNil() {
			return compareValues(expectedValue.Elem().Interface(), actualValue.Elem().Interface(), config)
		}

	case reflect.Interface:
		if expectedValue.IsNil() != actualValue.IsNil() {
			return fmt.Sprintf("nil mismatch: expected %v, got %v", expectedValue.IsNil(), actualValue.IsNil())
		}
		if !expectedValue.IsNil() {
			return compareValues(expectedValue.Elem().Interface(), actualValue.Elem().Interface(), config)
		}
	}

	return ""
}

// isZero returns whether a value is zero
func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.String:
		return v.String() == ""
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			return v.Interface().(time.Time).IsZero()
		}
		return reflect.DeepEqual(v.Interface(), reflect.Zero(v.Type()).Interface())
	case reflect.Map, reflect.Slice, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	}
	return false
}

// contains returns whether a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// CompareBuilder helps build comparison configuration
type CompareBuilder struct {
	t      *testing.T
	config CompareConfig
}

// NewCompareBuilder creates a new comparison builder
func NewCompareBuilder(t *testing.T) *CompareBuilder {
	return &CompareBuilder{
		t:      t,
		config: DefaultCompareConfig(),
	}
}

// WithFloatTolerance sets the float comparison tolerance
func (b *CompareBuilder) WithFloatTolerance(tolerance float64) *CompareBuilder {
	b.config.FloatTolerance = tolerance
	return b
}

// WithTimeTolerance sets the time comparison tolerance
func (b *CompareBuilder) WithTimeTolerance(tolerance time.Duration) *CompareBuilder {
	b.config.TimeTolerance = tolerance
	return b
}

// WithIgnoreFields sets the fields to ignore
func (b *CompareBuilder) WithIgnoreFields(fields ...string) *CompareBuilder {
	b.config.IgnoreFields = fields
	return b
}

// WithIgnoreZeroValues sets whether to ignore zero values
func (b *CompareBuilder) WithIgnoreZeroValues(ignore bool) *CompareBuilder {
	b.config.IgnoreZeroValues = ignore
	return b
}

// WithIgnoreCase sets whether to ignore case in string comparisons
func (b *CompareBuilder) WithIgnoreCase(ignore bool) *CompareBuilder {
	b.config.IgnoreCase = ignore
	return b
}

// Compare compares two values with the built configuration
func (b *CompareBuilder) Compare(expected, actual interface{}) {
	Compare(b.t, expected, actual, b.config)
}
