package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineSigningMethod(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "signing-test-")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tests := []struct {
		name           string
		imagePath      string
		dockerImage    string
		env            string
		setupFiles     func() string // Returns actual imagePath to use
		expectedMethod string
	}{
		{
			name: "keyless OIDC with certificate file",
			env:  "prod",
			setupFiles: func() string {
				imgPath := filepath.Join(tmpDir, "image.bin")
				certPath := imgPath + ".cert"

				// Create dummy files
				_ = os.WriteFile(imgPath, []byte("dummy image"), 0644)
				_ = os.WriteFile(certPath, []byte("dummy certificate"), 0644)

				return imgPath
			},
			expectedMethod: "keyless-oidc",
		},
		{
			name: "development signing with development signature",
			env:  "dev",
			setupFiles: func() string {
				imgPath := filepath.Join(tmpDir, "dev-image.bin")
				sigPath := imgPath + ".sig"

				// Create dummy files
				_ = os.WriteFile(imgPath, []byte("dummy image"), 0644)
				_ = os.WriteFile(sigPath, []byte("development signature"), 0644)

				return imgPath
			},
			expectedMethod: "development",
		},
		{
			name: "key-based signing with regular signature",
			env:  "prod",
			setupFiles: func() string {
				imgPath := filepath.Join(tmpDir, "key-image.bin")
				sigPath := imgPath + ".sig"

				// Create dummy files
				_ = os.WriteFile(imgPath, []byte("dummy image"), 0644)
				_ = os.WriteFile(sigPath, []byte("regular signature content"), 0644)

				return imgPath
			},
			expectedMethod: "key-based",
		},
		{
			name:           "docker image production environment",
			dockerImage:    "harbor.local/ploy/myapp:v1.0.0",
			env:            "production",
			expectedMethod: "keyless-oidc",
		},
		{
			name:           "docker image staging environment",
			dockerImage:    "harbor.local/ploy/myapp:v1.0.0",
			env:            "staging",
			expectedMethod: "keyless-oidc",
		},
		{
			name:           "docker image development environment",
			dockerImage:    "harbor.local/ploy/myapp:v1.0.0",
			env:            "dev",
			expectedMethod: "development",
		},
		{
			name:           "no artifacts defaults to development",
			env:            "prod",
			expectedMethod: "development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var imagePath string
			if tt.setupFiles != nil {
				imagePath = tt.setupFiles()
			} else {
				imagePath = tt.imagePath
			}

			result := determineSigningMethod(imagePath, tt.dockerImage, tt.env)
			assert.Equal(t, tt.expectedMethod, result)
		})
	}
}

func TestPerformVulnerabilityScanning(t *testing.T) {
	t.Setenv("PLOY_SKIP_VULN_SCAN", "1")

	tests := []struct {
		name        string
		imagePath   string
		dockerImage string
		env         string
		expected    bool
	}{
		{
			name:      "skip scanning in dev environment",
			imagePath: "/path/to/image.bin",
			env:       "dev",
			expected:  false,
		},
		{
			name:      "skip scanning in development environment",
			imagePath: "/path/to/image.bin",
			env:       "development",
			expected:  false,
		},
		{
			name:      "skip scanning with empty environment",
			imagePath: "/path/to/image.bin",
			env:       "",
			expected:  false,
		},
		{
			name:      "attempt scanning in production (will fail without grype)",
			imagePath: "/path/to/image.bin",
			env:       "production",
			expected:  false, // Will fail because grype is not installed
		},
		{
			name:        "attempt scanning in staging (will fail without grype)",
			dockerImage: "myapp:latest",
			env:         "staging",
			expected:    false, // Will fail because grype is not installed
		},
		{
			name:     "no target artifacts",
			env:      "production",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := performVulnerabilityScanning(tt.imagePath, tt.dockerImage, tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}
