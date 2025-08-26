package build

import (
	"testing"
)

func TestGetLogs(t *testing.T) {
	t.Skip("Integration test - requires Nomad API mocking")
	// This test would require mocking the Nomad monitor
	// GetLogs function requires actual Nomad deployment to test properly
}