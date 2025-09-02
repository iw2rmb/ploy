package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSourceRepository(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "repo-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name        string
		setupFiles  func() string // Returns directory path to use
		expectedURL string
	}{
		{
			name: "extract from package.json with object repository",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "obj-repo")
				os.MkdirAll(dir, 0755)
				
				packageJSON := map[string]interface{}{
					"name": "test-app",
					"repository": map[string]interface{}{
						"type": "git",
						"url":  "https://github.com/user/repo.git",
					},
				}
				
				data, _ := json.Marshal(packageJSON)
				os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
				
				return dir
			},
			expectedURL: "https://github.com/user/repo.git",
		},
		{
			name: "extract from package.json with string repository",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "str-repo")
				os.MkdirAll(dir, 0755)
				
				packageJSON := map[string]interface{}{
					"name":       "test-app",
					"repository": "https://github.com/user/another-repo.git",
				}
				
				data, _ := json.Marshal(packageJSON)
				os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
				
				return dir
			},
			expectedURL: "https://github.com/user/another-repo.git",
		},
		{
			name: "no package.json returns empty",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "no-package")
				os.MkdirAll(dir, 0755)
				return dir
			},
			expectedURL: "",
		},
		{
			name: "package.json without repository returns empty",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "no-repo-field")
				os.MkdirAll(dir, 0755)
				
				packageJSON := map[string]interface{}{
					"name": "test-app",
					"version": "1.0.0",
				}
				
				data, _ := json.Marshal(packageJSON)
				os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
				
				return dir
			},
			expectedURL: "",
		},
		{
			name: "invalid package.json returns empty",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "invalid-json")
				os.MkdirAll(dir, 0755)
				
				os.WriteFile(filepath.Join(dir, "package.json"), []byte("invalid json"), 0644)
				
				return dir
			},
			expectedURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFiles()
			result := extractSourceRepository(dir)
			assert.Equal(t, tt.expectedURL, result)
		})
	}
}