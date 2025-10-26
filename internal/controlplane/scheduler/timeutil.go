package scheduler

import (
	"encoding/json"
	"strings"
	"time"
)

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func decodeTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func decodeKVMap(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil
	}
	if len(generic) == 0 {
		return nil
	}
	for key, value := range generic {
		if str, ok := value.(string); ok {
			generic[key] = strings.TrimSpace(str)
		}
	}
	return generic
}

func snapshotTimestamp(fields map[string]any, fallback time.Time) string {
	if len(fields) == 0 {
		return ""
	}
	for _, key := range []string{"heartbeat", "checked_at", "observed_at", "timestamp"} {
		if raw, ok := fields[key]; ok {
			if str, ok := raw.(string); ok {
				trimmed := strings.TrimSpace(str)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	if fallback.IsZero() {
		return ""
	}
	return encodeTime(fallback)
}
