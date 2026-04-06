package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTokenFromFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		perm        os.FileMode
		wantErr     bool
		errContains string
		wantToken   string
	}{
		{
			name:      "valid token file with 0600 permissions",
			content:   "glpat-abc123xyz",
			perm:      0600,
			wantToken: "glpat-abc123xyz",
		},
		{
			name:      "valid token with whitespace trimmed",
			content:   "  glpat-abc123xyz\n",
			perm:      0600,
			wantToken: "glpat-abc123xyz",
		},
		{
			name:      "valid token with 0400 permissions (read-only)",
			content:   "glpat-abc123xyz",
			perm:      0400,
			wantToken: "glpat-abc123xyz",
		},
		{
			name:        "insecure permissions 0644",
			content:     "glpat-abc123xyz",
			perm:        0644,
			wantErr:     true,
			errContains: "insecure permissions",
		},
		{
			name:        "insecure permissions 0666",
			content:     "glpat-abc123xyz",
			perm:        0666,
			wantErr:     true,
			errContains: "insecure permissions",
		},
		{
			name:        "empty token file",
			content:     "",
			perm:        0600,
			wantErr:     true,
			errContains: "is empty",
		},
		{
			name:        "whitespace only token file",
			content:     "   \n\t\n",
			perm:        0600,
			wantErr:     true,
			errContains: "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tokenPath := filepath.Join(tmpDir, "token")

			if err := os.WriteFile(tokenPath, []byte(tt.content), tt.perm); err != nil {
				t.Fatalf("failed to create test token file: %v", err)
			}

			got, err := loadTokenFromFile(tokenPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("loadTokenFromFile() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("loadTokenFromFile() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("loadTokenFromFile() unexpected error: %v", err)
				return
			}
			if got != tt.wantToken {
				t.Errorf("loadTokenFromFile() = %q, want %q", got, tt.wantToken)
			}
		})
	}
}

func TestLoadTokenFromFile_NonExistent(t *testing.T) {
	_, err := loadTokenFromFile("/nonexistent/path/token")
	if err == nil {
		t.Error("loadTokenFromFile() expected error for non-existent file, got nil")
	}
}
