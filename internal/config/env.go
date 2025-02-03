package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvLoader provides type-safe environment variable loading with validation
type EnvLoader struct {
	prefix string
	vars   map[string]string
}

// NewEnvLoader creates a new environment variable loader with the given prefix
func NewEnvLoader(prefix string) *EnvLoader {
	return &EnvLoader{
		prefix: prefix,
		vars:   make(map[string]string),
	}
}

// LoadAll loads all environment variables with the configured prefix
func (e *EnvLoader) LoadAll() {
	for _, env := range os.Environ() {
		if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
			key := parts[0]
			if strings.HasPrefix(key, e.prefix) {
				e.vars[key] = parts[1]
			}
		}
	}
}

// GetString returns a string value from environment variables
func (e *EnvLoader) GetString(key string, defaultValue string) string {
	fullKey := e.prefix + key
	if val, ok := e.vars[fullKey]; ok {
		return val
	}
	return defaultValue
}

// GetInt returns an integer value from environment variables
func (e *EnvLoader) GetInt(key string, defaultValue int) (int, error) {
	if val := e.GetString(key, ""); val != "" {
		return strconv.Atoi(val)
	}
	return defaultValue, nil
}

// GetUint16 returns a uint16 value from environment variables
func (e *EnvLoader) GetUint16(key string, defaultValue uint16) (uint16, error) {
	if val := e.GetString(key, ""); val != "" {
		n, err := strconv.ParseUint(val, 10, 16)
		if err != nil {
			return 0, fmt.Errorf("invalid uint16 value for %s: %w", key, err)
		}
		return uint16(n), nil
	}
	return defaultValue, nil
}

// GetBool returns a boolean value from environment variables
func (e *EnvLoader) GetBool(key string, defaultValue bool) bool {
	if val := e.GetString(key, ""); val != "" {
		return strings.ToLower(val) == "true" || val == "1"
	}
	return defaultValue
}

// GetDuration returns a duration value from environment variables
func (e *EnvLoader) GetDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	if val := e.GetString(key, ""); val != "" {
		return time.ParseDuration(val)
	}
	return defaultValue, nil
}

// GetFloat64 returns a float64 value from environment variables
func (e *EnvLoader) GetFloat64(key string, defaultValue float64) (float64, error) {
	if val := e.GetString(key, ""); val != "" {
		return strconv.ParseFloat(val, 64)
	}
	return defaultValue, nil
}

// Required ensures that a required environment variable is set
func (e *EnvLoader) Required(key string) (string, error) {
	fullKey := e.prefix + key
	if val, ok := e.vars[fullKey]; ok && val != "" {
		return val, nil
	}
	return "", fmt.Errorf("required environment variable %s not set", fullKey)
}

// Validate checks if a value meets certain validation criteria
type Validate func(string) error

// GetStringValidated returns a validated string value from environment variables
func (e *EnvLoader) GetStringValidated(key string, defaultValue string, validators ...Validate) (string, error) {
	val := e.GetString(key, defaultValue)
	for _, validate := range validators {
		if err := validate(val); err != nil {
			return "", fmt.Errorf("validation failed for %s: %w", key, err)
		}
	}
	return val, nil
}

// Common validators
var (
	ValidateNotEmpty = func(val string) error {
		if val == "" {
			return fmt.Errorf("value cannot be empty")
		}
		return nil
	}

	ValidatePort = func(val string) error {
		port, err := strconv.ParseUint(val, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid port number")
		}
		if port < MinPort || port > MaxPort {
			return fmt.Errorf("port must be between %d and %d", MinPort, MaxPort)
		}
		return nil
	}

	ValidateSS58Address = func(val string) error {
		if !isValidSS58Address(val) {
			return fmt.Errorf("invalid SS58 address format")
		}
		return nil
	}
)
