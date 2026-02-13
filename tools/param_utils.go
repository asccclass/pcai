package tools

import (
	"fmt"
)

// ToString extracts a string value from a potential complex object (e.g. {"value":"..."})
func ToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		// Check for {"value": "..."} or {"type": "...", "value": "..."}
		if v, ok := val["value"]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	// Fallback to default string representation
	return fmt.Sprintf("%v", v)
}

// ToStringSlice extracts a string slice from a potential complex object or single string
func ToStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	// If it's explicitly a []interface{} (from JSON array)
	if slice, ok := v.([]interface{}); ok {
		result := make([]string, len(slice))
		for i, item := range slice {
			result[i] = ToString(item)
		}
		return result
	} else if mapVal, ok := v.(map[string]interface{}); ok {
		// Handle wrapped array: {"type":"array", "value":["a", "b"]}
		if val, ok := mapVal["value"]; ok {
			return ToStringSlice(val) // Recursive call on inner value
		}
	} else if strVal, ok := v.(string); ok {
		// Single string treated as one-element slice? Or maybe parsing error?
		// Usually args are explicit arrays. If string, wrap it.
		return []string{strVal}
	}

	return []string{}
}
