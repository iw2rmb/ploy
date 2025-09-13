package vps

import "testing"

// This stub keeps the package buildable for default unit runs.
func TestVPS_Skipped(t *testing.T) {
	t.Skip("VPS tests run only with -tags vps")
}
