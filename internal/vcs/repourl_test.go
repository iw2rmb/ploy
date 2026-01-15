package vcs

import (
	"testing"
)

// TestNormalizeRepoURL is the comprehensive test suite for NormalizeRepoURL.
// This covers edge cases around trailing slash, .git suffix combinations, and whitespace
// to ensure consistent URL normalization for cache key generation and repo URL matching.
func TestNormalizeRepoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cases: trailing .git suffix removal.
		{
			name:     "removes trailing .git suffix",
			input:    "https://github.com/org/repo.git",
			expected: "https://github.com/org/repo",
		},

		// Basic cases: trailing slash removal.
		{
			name:     "removes trailing slash",
			input:    "https://github.com/org/repo/",
			expected: "https://github.com/org/repo",
		},

		// Combination cases: both trailing slash and .git suffix.
		{
			name:     "removes trailing slash then .git (slash after .git)",
			input:    "https://github.com/org/repo.git/",
			expected: "https://github.com/org/repo",
		},

		// Whitespace handling.
		{
			name:     "trims leading and trailing whitespace",
			input:    "  https://github.com/org/repo  ",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "trims tabs",
			input:    "\thttps://github.com/org/repo\t",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "trims mixed whitespace",
			input:    " \t https://github.com/org/repo \t ",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "trims newlines",
			input:    "\nhttps://github.com/org/repo\n",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "combined whitespace and .git suffix",
			input:    "  https://github.com/org/repo.git  ",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "combined whitespace, .git suffix, and trailing slash",
			input:    "  https://github.com/org/repo.git/  ",
			expected: "https://github.com/org/repo",
		},

		// SSH URL variants.
		{
			name:     "SSH URL with .git suffix",
			input:    "git@github.com:org/repo.git",
			expected: "git@github.com:org/repo",
		},
		{
			name:     "SSH URL without .git suffix",
			input:    "git@github.com:org/repo",
			expected: "git@github.com:org/repo",
		},
		{
			name:     "SSH URL with trailing slash",
			input:    "git@github.com:org/repo/",
			expected: "git@github.com:org/repo",
		},
		{
			name:     "ssh:// scheme URL with .git suffix",
			input:    "ssh://git@github.com/org/repo.git",
			expected: "ssh://git@github.com/org/repo",
		},

		// HTTPS URL variants.
		{
			name:     "HTTPS URL without modifications",
			input:    "https://github.com/org/repo",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "HTTPS URL with port",
			input:    "https://github.com:443/org/repo.git",
			expected: "https://github.com:443/org/repo",
		},

		// file:// URL variants.
		{
			name:     "file:// URL with .git suffix",
			input:    "file:///path/to/repo.git",
			expected: "file:///path/to/repo",
		},
		{
			name:     "file:// URL with trailing slash",
			input:    "file:///path/to/repo/",
			expected: "file:///path/to/repo",
		},
		{
			name:     "file:// URL clean",
			input:    "file:///path/to/repo",
			expected: "file:///path/to/repo",
		},

		// Different git hosting providers.
		{
			name:     "GitLab URL with .git suffix",
			input:    "https://gitlab.example.com/org/repo.git",
			expected: "https://gitlab.example.com/org/repo",
		},
		{
			name:     "Bitbucket URL with .git suffix",
			input:    "https://bitbucket.org/org/repo.git",
			expected: "https://bitbucket.org/org/repo",
		},
		{
			name:     "self-hosted GitLab with port",
			input:    "https://git.internal.example.com:8443/org/project.git",
			expected: "https://git.internal.example.com:8443/org/project",
		},

		// Edge cases: empty and whitespace-only inputs.
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace-only string (spaces)",
			input:    "   ",
			expected: "",
		},
		{
			name:     "whitespace-only string (tabs)",
			input:    "\t\t\t",
			expected: "",
		},
		{
			name:     "whitespace-only string (mixed)",
			input:    " \t \n ",
			expected: "",
		},

		// Edge cases: URLs with .git in the path (not as suffix).
		{
			name:     "URL with .git in middle of path preserved",
			input:    "https://github.com/org/.git-templates/repo",
			expected: "https://github.com/org/.git-templates/repo",
		},
		{
			name:     "URL with .git in repo name (not suffix)",
			input:    "https://github.com/org/my.git.repo",
			expected: "https://github.com/org/my.git.repo",
		},

		// Edge cases: multiple trailing slashes (only one is removed).
		{
			name:     "multiple trailing slashes (one removed)",
			input:    "https://github.com/org/repo//",
			expected: "https://github.com/org/repo/",
		},

		// Edge cases: URLs that end with just ".git" (e.g., repo named ".git").
		{
			name:     "URL ending with path segment .git",
			input:    "https://github.com/org/.git",
			expected: "https://github.com/org/",
		},

		// Deep paths.
		{
			name:     "deep nested path with .git suffix",
			input:    "https://github.com/org/team/subteam/repo.git",
			expected: "https://github.com/org/team/subteam/repo",
		},
		{
			name:     "deep nested path with trailing slash",
			input:    "https://github.com/org/team/subteam/repo/",
			expected: "https://github.com/org/team/subteam/repo",
		},

		// Query strings and fragments (should be preserved except for suffixes).
		{
			name:     "URL with query string preserved",
			input:    "https://github.com/org/repo?ref=main",
			expected: "https://github.com/org/repo?ref=main",
		},
		{
			name:     "URL with fragment preserved",
			input:    "https://github.com/org/repo#readme",
			expected: "https://github.com/org/repo#readme",
		},
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

// TestNormalizeRepoURL_Consistency verifies that equivalent URLs all normalize
// to the same canonical form. This is critical for cache key generation and
// repo URL matching across the codebase.
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
				"  https://github.com/org/repo.git  ",
				"\thttps://github.com/org/repo.git/\t",
			},
			expectedResult: "https://github.com/org/repo",
		},
		{
			name: "SSH variants (SCP-style)",
			equivalentURLs: []string{
				"git@github.com:org/repo",
				"git@github.com:org/repo.git",
				"  git@github.com:org/repo  ",
				"  git@github.com:org/repo.git  ",
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

// TestNormalizeRepoURL_Idempotent verifies that applying NormalizeRepoURL
// multiple times produces the same result (the function is idempotent).
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
			thirdPass := NormalizeRepoURL(secondPass)

			if firstPass != secondPass {
				t.Errorf("NormalizeRepoURL is not idempotent: first=%q, second=%q", firstPass, secondPass)
			}
			if secondPass != thirdPass {
				t.Errorf("NormalizeRepoURL is not idempotent: second=%q, third=%q", secondPass, thirdPass)
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
		{
			name:     "https without .git",
			input:    "https://github.com/org/repo",
			expected: "github.com/org/repo",
		},
		{
			name:     "https with .git",
			input:    "https://github.com/org/repo.git",
			expected: "github.com/org/repo",
		},
		{
			name:     "ssh scheme with user",
			input:    "ssh://git@github.com/org/repo.git",
			expected: "github.com/org/repo",
		},
		{
			name:     "scp style with user",
			input:    "git@github.com:org/repo.git",
			expected: "github.com/org/repo",
		},
		{
			name:     "https with port retains port",
			input:    "https://github.com:443/org/repo.git",
			expected: "github.com:443/org/repo",
		},
		{
			name:     "file scheme becomes path",
			input:    "file:///path/to/repo.git",
			expected: "/path/to/repo",
		},
		{
			name:     "empty",
			input:    "   ",
			expected: "",
		},
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
