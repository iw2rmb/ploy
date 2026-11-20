package common

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestFormatRunListRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		url    string
		sha    string
		expect string
	}{
		{
			name:   "https repo with sha",
			url:    "https://gitlab.example.com/team/service.git",
			sha:    "0123456789abcdef0123456789abcdef01234567",
			expect: "team/service:01234567",
		},
		{
			name:   "ssh repo keeps namespace path",
			url:    "git@gitlab.example.com:platform/team/service.git",
			sha:    "abcdef1234567890abcdef1234567890abcdef12",
			expect: "platform/team/service:abcdef12",
		},
		{
			name:   "missing sha omits suffix",
			url:    "https://github.com/org/repo.git",
			sha:    "",
			expect: "org/repo",
		},
		{
			name:   "missing repo",
			url:    "",
			sha:    "0123456789abcdef0123456789abcdef01234567",
			expect: "-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatRunListRepo(tc.url, tc.sha); got != tc.expect {
				t.Fatalf("FormatRunListRepo(%q, %q) = %q, want %q", tc.url, tc.sha, got, tc.expect)
			}
		})
	}
}

func TestFormatRunListSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		summary domaintypes.RunSummary
		want    string
	}{
		{
			name: "named spec uses source and name",
			summary: domaintypes.RunSummary{
				SpecID: "spec-1", SpecSourceDomain: " gitlab.example.com/ ",
				SpecSourceRepo: " /team/specs/ ", SpecName: " upgrade ",
			},
			want: "gitlab.example.com/team/specs:upgrade",
		},
		{
			name:    "anonymous spec uses ID",
			summary: domaintypes.RunSummary{SpecID: "spec-1"},
			want:    "spec-1",
		},
		{
			name:    "incomplete named spec uses ID",
			summary: domaintypes.RunSummary{SpecID: "spec-1", SpecName: "upgrade"},
			want:    "spec-1",
		},
		{
			name: "missing spec uses placeholder",
			want: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatRunListSpec(tt.summary); got != tt.want {
				t.Fatalf("FormatRunListSpec() = %q, want %q", got, tt.want)
			}
		})
	}
}
