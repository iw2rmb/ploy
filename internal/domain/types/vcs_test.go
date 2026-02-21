package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRepoURL_AcceptsAndNormalizes(t *testing.T) {
	tests := []struct{ in, want string }{
		{"  https://github.com/acme/repo.git  ", "https://github.com/acme/repo.git"},
		{" ssh://git@github.com/acme/repo.git ", "ssh://git@github.com/acme/repo.git"},
		{" file:///var/tmp/repo ", "file:///var/tmp/repo"},
	}
	for _, tt := range tests {
		var v RepoURL
		if err := v.UnmarshalText([]byte(tt.in)); err != nil {
			t.Fatalf("unmarshal %q: %v", tt.in, err)
		}
		if v.String() != tt.want {
			t.Fatalf("normalize: got %q want %q", v.String(), tt.want)
		}
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		var v2 RepoURL
		if err := json.Unmarshal(b, &v2); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if v2 != v {
			t.Fatalf("roundtrip mismatch: %q != %q", v2, v)
		}
	}
}

func TestRepoURL_RejectsEmpty(t *testing.T) {
	var v RepoURL
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestGitRef_TrimAndJSON(t *testing.T) {
	var r GitRef
	if err := r.UnmarshalText([]byte("  main  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if r.String() != "main" {
		t.Fatalf("normalize got %q", r.String())
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var r2 GitRef
	if err := json.Unmarshal(b, &r2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if r2 != r {
		t.Fatalf("roundtrip mismatch: %q != %q", r2, r)
	}
}

func TestCommitSHA_TrimAndJSON(t *testing.T) {
	var c CommitSHA
	if err := c.UnmarshalText([]byte("  abcdef1  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if c.String() != "abcdef1" {
		t.Fatalf("normalize got %q", c.String())
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var c2 CommitSHA
	if err := json.Unmarshal(b, &c2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if c2 != c {
		t.Fatalf("roundtrip mismatch: %q != %q", c2, c)
	}
}

func TestNormalizeRepoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "removes trailing .git suffix", input: "https://github.com/org/repo.git", expected: "https://github.com/org/repo"},
		{name: "removes trailing slash", input: "https://github.com/org/repo/", expected: "https://github.com/org/repo"},
		{name: "removes trailing slash then .git", input: "https://github.com/org/repo.git/", expected: "https://github.com/org/repo"},
		{name: "trims whitespace", input: "  https://github.com/org/repo  ", expected: "https://github.com/org/repo"},
		{name: "trims tabs", input: "\thttps://github.com/org/repo\t", expected: "https://github.com/org/repo"},
		{name: "combined whitespace and .git", input: "  https://github.com/org/repo.git  ", expected: "https://github.com/org/repo"},
		{name: "combined whitespace .git and slash", input: "  https://github.com/org/repo.git/  ", expected: "https://github.com/org/repo"},
		{name: "SSH URL with .git", input: "git@github.com:org/repo.git", expected: "git@github.com:org/repo"},
		{name: "SSH URL without .git", input: "git@github.com:org/repo", expected: "git@github.com:org/repo"},
		{name: "ssh:// scheme with .git", input: "ssh://git@github.com/org/repo.git", expected: "ssh://git@github.com/org/repo"},
		{name: "HTTPS unmodified", input: "https://github.com/org/repo", expected: "https://github.com/org/repo"},
		{name: "HTTPS with port", input: "https://github.com:443/org/repo.git", expected: "https://github.com:443/org/repo"},
		{name: "file:// with .git", input: "file:///path/to/repo.git", expected: "file:///path/to/repo"},
		{name: "file:// with slash", input: "file:///path/to/repo/", expected: "file:///path/to/repo"},
		{name: "empty string", input: "", expected: ""},
		{name: "whitespace-only", input: "   ", expected: ""},
		{name: ".git in middle preserved", input: "https://github.com/org/.git-templates/repo", expected: "https://github.com/org/.git-templates/repo"},
		{name: "multiple trailing slashes", input: "https://github.com/org/repo//", expected: "https://github.com/org/repo/"},
		{name: "deep nested path with .git", input: "https://github.com/org/team/subteam/repo.git", expected: "https://github.com/org/team/subteam/repo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := NormalizeRepoURL(tc.input)
			if result != tc.expected {
				t.Errorf("NormalizeRepoURL(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestNormalizeRepoURL_Consistency(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		equivalentURLs []string
		expectedResult string
	}{
		{
			name: "HTTPS variants",
			equivalentURLs: []string{
				"https://github.com/org/repo",
				"https://github.com/org/repo/",
				"https://github.com/org/repo.git",
				"https://github.com/org/repo.git/",
				"  https://github.com/org/repo  ",
			},
			expectedResult: "https://github.com/org/repo",
		},
		{
			name: "SSH variants",
			equivalentURLs: []string{
				"git@github.com:org/repo",
				"git@github.com:org/repo.git",
				"  git@github.com:org/repo  ",
			},
			expectedResult: "git@github.com:org/repo",
		},
		{
			name: "file:// variants",
			equivalentURLs: []string{
				"file:///home/user/repo",
				"file:///home/user/repo/",
				"file:///home/user/repo.git",
				"file:///home/user/repo.git/",
			},
			expectedResult: "file:///home/user/repo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, url := range tc.equivalentURLs {
				result := NormalizeRepoURL(url)
				if result != tc.expectedResult {
					t.Errorf("NormalizeRepoURL(%q) = %q, expected %q", url, result, tc.expectedResult)
				}
			}
		})
	}
}

func TestNormalizeRepoURL_Idempotent(t *testing.T) {
	t.Parallel()

	testURLs := []string{
		"https://github.com/org/repo.git/",
		"  https://github.com/org/repo  ",
		"git@github.com:org/repo.git",
		"file:///path/to/repo.git/",
		"",
		"   ",
	}

	for _, url := range testURLs {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			firstPass := NormalizeRepoURL(url)
			secondPass := NormalizeRepoURL(firstPass)
			if firstPass != secondPass {
				t.Errorf("not idempotent: first=%q, second=%q", firstPass, secondPass)
			}
		})
	}
}

func TestNormalizeRepoURLSchemless(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "https without .git", input: "https://github.com/org/repo", expected: "github.com/org/repo"},
		{name: "https with .git", input: "https://github.com/org/repo.git", expected: "github.com/org/repo"},
		{name: "ssh scheme with user", input: "ssh://git@github.com/org/repo.git", expected: "github.com/org/repo"},
		{name: "scp style with user", input: "git@github.com:org/repo.git", expected: "github.com/org/repo"},
		{name: "https with port", input: "https://github.com:443/org/repo.git", expected: "github.com:443/org/repo"},
		{name: "file scheme", input: "file:///path/to/repo.git", expected: "/path/to/repo"},
		{name: "empty", input: "   ", expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeRepoURLSchemless(tc.input)
			if got != tc.expected {
				t.Fatalf("NormalizeRepoURLSchemless(%q)=%q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
