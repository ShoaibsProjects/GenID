package connector

import (
	"regexp"
	"strconv"
	"strings"
)

// GetField safely retrieves nested values from a map using a dot-separated path
// and simple array indices (e.g., 'name[0].given[0]').
// Note: Does not support complex JSONPath filters like [?(@.x==1)] in V1.
func GetField(data map[string]interface{}, path string) interface{} {
	var current interface{} = data
	segments := strings.Split(path, ".")

	for _, segment := range segments {
		re := regexp.MustCompile(`^([^\[]+)(\[([0-9]+)\])?$`)
		matches := re.FindStringSubmatch(segment)

		if len(matches) == 0 {
			return nil
		}

		key := matches[1]
		indexStr := matches[3]

		if key != "" {
			if m, ok := current.(map[string]interface{}); ok {
				if val, exists := m[key]; exists {
					current = val
				} else {
					return nil
				}
			} else {
				return nil
			}
		}

		if indexStr != "" {
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil
			}
			if arr, ok := current.([]interface{}); ok {
				if index >= 0 && index < len(arr) {
					current = arr[index]
				} else {
					return nil
				}
			} else {
				return nil
			}
		}
	}
	return current
}