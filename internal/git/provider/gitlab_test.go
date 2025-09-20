package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGitLabProvider_CreateOrUpdateMR(t *testing.T) {
	tests := []struct {
		name           string
		config         MRConfig
		envURL         string
		envToken       string
		mockResponse   string
		mockStatus     int
		existingMRs    []gitLabMR
		expectedResult *MRResult
		expectError    bool
	}{
		{
			name: "creates new MR successfully",
			config: MRConfig{
				RepoURL:      "https://gitlab.example.com/namespace/project.git",
				SourceBranch: "workflow/java17-migration/20250905",
				TargetBranch: "refs/heads/main",
				Title:        "Transflow: Java 17 Migration",
				Description:  "Applied OpenRewrite recipe for Java 17 migration",
				Labels:       []string{"ploy", "tfl"},
			},
			envURL:   "https://gitlab.example.com",
			envToken: "glpat-test-token",
			mockResponse: `{
				"id": 123,
				"iid": 1,
				"web_url": "https://gitlab.example.com/namespace/project/-/merge_requests/1",
				"title": "Transflow: Java 17 Migration"
			}`,
			mockStatus:  201,
			existingMRs: []gitLabMR{}, // no existing MRs
			expectedResult: &MRResult{
				MRURL:   "https://gitlab.example.com/namespace/project/-/merge_requests/1",
				MRID:    1,
				Created: true,
			},
			expectError: false,
		},
		{
			name: "updates existing MR successfully",
			config: MRConfig{
				RepoURL:      "https://gitlab.example.com/namespace/project.git",
				SourceBranch: "workflow/java17-migration/20250905",
				TargetBranch: "refs/heads/main",
				Title:        "Transflow: Java 17 Migration (Updated)",
				Description:  "Updated: Applied OpenRewrite recipe for Java 17 migration with healing",
				Labels:       []string{"ploy", "tfl"},
			},
			envURL:   "https://gitlab.example.com",
			envToken: "glpat-test-token",
			mockResponse: `{
				"id": 123,
				"iid": 1,
				"web_url": "https://gitlab.example.com/namespace/project/-/merge_requests/1",
				"title": "Transflow: Java 17 Migration (Updated)"
			}`,
			mockStatus: 200,
			existingMRs: []gitLabMR{
				{
					ID:           123,
					IID:          1,
					SourceBranch: "workflow/java17-migration/20250905",
					TargetBranch: "main",
					State:        "opened",
				},
			},
			expectedResult: &MRResult{
				MRURL:   "https://gitlab.example.com/namespace/project/-/merge_requests/1",
				MRID:    1,
				Created: false,
			},
			expectError: false,
		},
		{
			name: "fails with invalid repository URL",
			config: MRConfig{
				RepoURL:      "not-a-valid-url",
				SourceBranch: "workflow/test/20250905",
				TargetBranch: "refs/heads/main",
				Title:        "Test MR",
				Description:  "Test description",
				Labels:       []string{"ploy"},
			},
			envURL:      "https://gitlab.example.com",
			envToken:    "glpat-test-token",
			expectError: true,
		},
		{
			name: "fails with missing GitLab token",
			config: MRConfig{
				RepoURL:      "https://gitlab.example.com/namespace/project.git",
				SourceBranch: "workflow/test/20250905",
				TargetBranch: "refs/heads/main",
				Title:        "Test MR",
				Description:  "Test description",
				Labels:       []string{"ploy"},
			},
			envURL:      "https://gitlab.example.com",
			envToken:    "", // empty token
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			if tt.envURL != "" {
				_ = os.Setenv("GITLAB_URL", tt.envURL)
			} else {
				_ = os.Unsetenv("GITLAB_URL")
			}

			if tt.envToken != "" {
				_ = os.Setenv("PLOY_GITLAB_PAT", tt.envToken)
			} else {
				_ = os.Unsetenv("PLOY_GITLAB_PAT")
			}

			defer func() {
				_ = os.Unsetenv("GITLAB_URL")
				_ = os.Unsetenv("PLOY_GITLAB_PAT")
			}()

			// Create mock GitLab server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check authorization header
				if tt.envToken != "" {
					expectedAuth := "Bearer " + tt.envToken
					if r.Header.Get("Authorization") != expectedAuth {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
				}

				// Handle different API endpoints
				if strings.Contains(r.URL.Path, "/merge_requests") && r.Method == "GET" {
					// List existing MRs
					response, _ := json.Marshal(tt.existingMRs)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(response)
					return
				}

				if strings.Contains(r.URL.Path, "/merge_requests") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.mockStatus)
					_, _ = w.Write([]byte(tt.mockResponse))
					return
				}

				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			// Override GitLab URL to use mock server
			if tt.envURL != "" {
				_ = os.Setenv("GITLAB_URL", server.URL)
			}

			// Create GitLab provider
			provider := NewGitLabProvider()

			// Test CreateOrUpdateMR
			ctx := context.Background()
			result, err := provider.CreateOrUpdateMR(ctx, tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected result but got nil")
				return
			}

			if result.MRURL != tt.expectedResult.MRURL {
				t.Errorf("expected MR URL %s, got %s", tt.expectedResult.MRURL, result.MRURL)
			}

			if result.MRID != tt.expectedResult.MRID {
				t.Errorf("expected MR ID %d, got %d", tt.expectedResult.MRID, result.MRID)
			}

			if result.Created != tt.expectedResult.Created {
				t.Errorf("expected Created %t, got %t", tt.expectedResult.Created, result.Created)
			}
		})
	}
}

func TestGitLabProvider_ValidateConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		envURL      string
		envToken    string
		expectError bool
	}{
		{
			name:        "valid configuration",
			envURL:      "https://gitlab.example.com",
			envToken:    "glpat-test-token",
			expectError: false,
		},
		{
			name:        "missing GitLab token",
			envURL:      "https://gitlab.example.com",
			envToken:    "",
			expectError: true,
		},
		{
			name:        "uses default GitLab URL when not specified",
			envURL:      "",
			envToken:    "glpat-test-token",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			if tt.envURL != "" {
				_ = os.Setenv("GITLAB_URL", tt.envURL)
			} else {
				_ = os.Unsetenv("GITLAB_URL")
			}

			if tt.envToken != "" {
				_ = os.Setenv("GITLAB_TOKEN", tt.envToken)
			} else {
				_ = os.Unsetenv("GITLAB_TOKEN")
			}

			defer func() {
				_ = os.Unsetenv("GITLAB_URL")
				_ = os.Unsetenv("GITLAB_TOKEN")
			}()

			provider := NewGitLabProvider()
			err := provider.ValidateConfiguration()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestExtractGitLabProject(t *testing.T) {
	tests := []struct {
		name        string
		repoURL     string
		expected    string
		expectError bool
	}{
		{
			name:        "standard GitLab HTTPS URL",
			repoURL:     "https://gitlab.example.com/namespace/project.git",
			expected:    "namespace/project",
			expectError: false,
		},
		{
			name:        "GitLab URL without .git suffix",
			repoURL:     "https://gitlab.example.com/namespace/project",
			expected:    "namespace/project",
			expectError: false,
		},
		{
			name:        "nested namespace",
			repoURL:     "https://gitlab.example.com/group/subgroup/project.git",
			expected:    "group/subgroup/project",
			expectError: false,
		},
		{
			name:        "invalid URL",
			repoURL:     "not-a-valid-url",
			expected:    "",
			expectError: true,
		},
		{
			name:        "GitHub URL (valid format but not GitLab)",
			repoURL:     "https://github.com/user/repo.git",
			expected:    "user/repo",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractGitLabProject(tt.repoURL)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// gitLabMR represents a GitLab merge request response from API
type gitLabMR struct {
	ID           int    `json:"id"`
	IID          int    `json:"iid"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	State        string `json:"state"`
	WebURL       string `json:"web_url,omitempty"`
}
