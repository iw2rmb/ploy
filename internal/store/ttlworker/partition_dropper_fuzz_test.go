package ttlworker

import (
	"testing"
)

// FuzzPartitionPattern asserts that the partition regex never panics and, when
// it matches, the captured month is always within 01..12 (enforced by regex).
// This target is intentionally small/fast to keep local CI lean.
func FuzzPartitionPattern(f *testing.F) {
	// Seed with a variety of shapes.
	seeds := []string{
		"ploy.logs_2025_10",
		"ploy.events_1999_01",
		"public.logs_2025_10",
		"ploy.logs_2025_00",
		"ploy.logs_2025_13",
		"ploy.logs_aaaa_bb",
		"ploy.node_metrics_2030_12",
		"ploy.artifact_bundles_2024_09",
		"",
		"random",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		m := partitionPattern.FindStringSubmatch(s)
		if len(m) == 0 {
			return
		}
		// m[3] is the month. With the tightened regex this is guaranteed to be
		// 01..12. The properties below ensure we don't regress.
		if len(m) != 4 {
			t.Fatalf("unexpected submatch length: got %d", len(m))
		}
		month := m[3]
		if !(month >= "01" && month <= "12") {
			t.Fatalf("regex matched invalid month: %q", month)
		}
	})
}
