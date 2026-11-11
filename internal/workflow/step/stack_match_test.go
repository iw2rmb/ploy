package step

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

func TestMatchStack(t *testing.T) {
	t.Parallel()

	release := func(v string) *string { return &v }
	evidence := []stackdetect.EvidenceItem{{Path: "pom.xml", Key: "maven.compiler.release", Value: "11"}}

	tests := []struct {
		name       string
		obs        stackdetect.Observation
		expect     contracts.StackExpectation
		wantMatch  bool
		wantReason string
	}{
		{
			name:      "exact match",
			obs:       stackdetect.Observation{Language: "java", Tool: "maven", Release: release("17")},
			expect:    contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			wantMatch: true,
		},
		{
			name:      "language and tool are case-insensitive",
			obs:       stackdetect.Observation{Language: " Java ", Tool: " MAVEN ", Release: release("17")},
			expect:    contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			wantMatch: true,
		},
		{
			name:      "release ignores surrounding whitespace",
			obs:       stackdetect.Observation{Language: "java", Tool: "maven", Release: release(" 17 ")},
			expect:    contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			wantMatch: true,
		},
		{
			name:       "release remains case-sensitive",
			obs:        stackdetect.Observation{Language: "java", Tool: "maven", Release: release("RC1")},
			expect:     contracts.StackExpectation{Language: "java", Tool: "maven", Release: "rc1"},
			wantReason: `stack mismatch: release: expected "rc1", detected "RC1"`,
		},
		{
			name:      "language wildcard",
			obs:       stackdetect.Observation{Language: "go", Tool: "go", Release: release("1.25")},
			expect:    contracts.StackExpectation{Tool: "go", Release: "1.25"},
			wantMatch: true,
		},
		{
			name:      "tool wildcard",
			obs:       stackdetect.Observation{Language: "java", Tool: "gradle", Release: release("17")},
			expect:    contracts.StackExpectation{Language: "java", Release: "17"},
			wantMatch: true,
		},
		{
			name:      "release wildcard",
			obs:       stackdetect.Observation{Language: "java", Tool: "maven", Release: release("21")},
			expect:    contracts.StackExpectation{Language: "java", Tool: "maven"},
			wantMatch: true,
		},
		{
			name:       "tool mismatch",
			obs:        stackdetect.Observation{Language: "java", Tool: "gradle", Release: release("17")},
			expect:     contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			wantReason: `stack mismatch: tool: expected "maven", detected "gradle"`,
		},
		{
			name:       "missing detected release",
			obs:        stackdetect.Observation{Language: "java", Tool: "maven"},
			expect:     contracts.StackExpectation{Language: "java", Release: "17"},
			wantReason: `stack mismatch: release: expected "17", detected "<nil>"`,
		},
		{
			name:       "multiple mismatches without evidence suffix",
			obs:        stackdetect.Observation{Language: "go", Tool: "go", Release: release("11"), Evidence: evidence},
			expect:     contracts.StackExpectation{Language: "java", Release: "17"},
			wantReason: `stack mismatch: language: expected "java", detected "go"; release: expected "17", detected "11"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			obs, expect := tc.obs, tc.expect
			matched, reason := matchStack(&obs, &expect)
			if matched != tc.wantMatch {
				t.Fatalf("matchStack() matched = %v, want %v (reason %q)", matched, tc.wantMatch, reason)
			}
			if reason != tc.wantReason {
				t.Fatalf("matchStack() reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}
