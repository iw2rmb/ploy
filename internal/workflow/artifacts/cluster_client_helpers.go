// cluster_client_helpers.go holds shared helper functions for ClusterClient.
package artifacts

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// firstNonZero returns the first non-zero integer in the provided list.
func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

// firstNonEmpty returns the first non-empty trimmed string in the provided list.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// asString coerces the input into a string when possible.
func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

// toInt64 converts supported numeric JSON representations into an int64.
func toInt64(value any) int64 {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return 0
		}
		num, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0
		}
		return num
	case float64:
		return int64(v)
	case json.Number:
		num, err := v.Int64()
		if err != nil {
			return 0
		}
		return num
	default:
		return 0
	}
}
