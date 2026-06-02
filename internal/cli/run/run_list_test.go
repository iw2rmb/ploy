package run

import "testing"

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
			if got := formatRunListRepo(tc.url, tc.sha); got != tc.expect {
				t.Fatalf("formatRunListRepo(%q, %q) = %q, want %q", tc.url, tc.sha, got, tc.expect)
			}
		})
	}
}
