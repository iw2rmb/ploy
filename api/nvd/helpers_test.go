package nvd

import (
	"testing"
)

func TestMapCVSSToSeverity(t *testing.T) {
	db := NewNVDDatabase()
	cases := []struct {
		score float64
		want  string
	}{
		{9.8, "CRITICAL"},
		{8.0, "HIGH"},
		{6.5, "MEDIUM"},
		{3.9, "LOW"},
		{0.0, "LOW"},
	}
	for _, c := range cases {
		if got := db.mapCVSSToSeverity(c.score); got != c.want {
			t.Fatalf("score %.1f => %s, want %s", c.score, got, c.want)
		}
	}
}
