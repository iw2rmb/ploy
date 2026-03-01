package stackdetect

import (
	"regexp"
	"testing"
)

func TestCanonicalizeVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		re       *regexp.Regexp
		fallback string
		want     string
	}{
		{
			name:     "returns first submatch",
			input:    " 1.76.0 ",
			re:       regexp.MustCompile(`^(\d+\.\d+)(?:\.\d+)?$`),
			fallback: "fallback",
			want:     "1.76",
		},
		{
			name:     "returns fallback on no match",
			input:    "stable",
			re:       regexp.MustCompile(`^(\d+\.\d+)(?:\.\d+)?$`),
			fallback: "fallback",
			want:     "fallback",
		},
		{
			name:     "trims input before matching",
			input:    "   release-17 ",
			re:       regexp.MustCompile(`^release-(\d+)$`),
			fallback: "fallback",
			want:     "17",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := canonicalizeVersion(tc.input, tc.re, tc.fallback)
			if got != tc.want {
				t.Fatalf("canonicalizeVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}
