package util

import (
	"encoding/json"
)

// ConvertMapToStruct converts a map[string]any to a struct using JSON marshaling
func ConvertMapToStruct(m map[string]any, v any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
