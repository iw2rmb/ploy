package e2e

import "testing"

// This stub keeps the package buildable for default unit runs.
func TestE2E_Skipped(t *testing.T) {
	t.Skip("E2E tests run only with -tags e2e")
}
