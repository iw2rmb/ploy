// registry_metrics.go defines helpers for recording registry request and payload metrics.
package httpserver

import (
	"strconv"
	"strings"
)

// recordRegistryRequest increments the per-resource request metrics.
func recordRegistryRequest(resource, method string, status int) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	registryRequestsTotal.WithLabelValues(resource, method, strconv.Itoa(status)).Inc()
}

// recordRegistryPayload adds the number of bytes transferred for registry payload operations.
func recordRegistryPayload(resource, operation string, bytesCopied int64) {
	if bytesCopied <= 0 {
		return
	}
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	registryPayloadBytes.WithLabelValues(resource, operation).Add(float64(bytesCopied))
}
