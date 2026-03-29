package integration

import "testing"

func skipDBIntegrationUnderCoverage(t *testing.T) {
	t.Helper()
	if testing.CoverMode() != "" {
		t.Skip("skipping DB integration test under coverage due shared non-isolated PLOY_TEST_PG_DSN")
	}
}
