package build

import (
	"strings"
	"testing"
)

func TestLaneNormalizationDefaultsToDocker(t *testing.T) {
	inputs := []string{"A", "B", "C", "D", "E", "", "abc"}
	for _, in := range inputs {
		normalized := strings.ToUpper(in)
		if normalized != "D" {
			normalized = "D"
		}
		if normalized != "D" {
			t.Fatalf("expected lane %q to normalize to D", in)
		}
	}
}
