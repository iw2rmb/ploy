package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	publicKeyPath := createTestPublicKey(t, tempDir)
	configPath := filepath.Join(tempDir, "test-config.yaml")
	
	configContent := `
service:
  name: "test-service"
  port: 8080

executable:
  path: "pylint"
  args: ["--output-format=json", "--reports=no"]
  timeout: "5m"

security:
  auth_method: "public_key"
  public_key_path: "` + publicKeyPath + `"
  run_as_user: "pylint"
  max_memory: "512MB"
  max_cpu: "1.0"

input:
  formats: ["tar.gz", "tar", "zip"]
  allowed_extensions: [".py", ".pyw"]
  max_archive_size: "100MB"

output:
  format: "json"
  parser: "pylint_json"
`
	
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	
	config, err := LoadConfig(configPath)
	require.NoError(t, err)
	
	// Verify service configuration
	assert.Equal(t, "test-service", config.Service.Name)
	assert.Equal(t, 8080, config.Service.Port)
	
	// Verify executable configuration
	assert.Equal(t, "pylint", config.Executable.Path)
	assert.Equal(t, []string{"--output-format=json", "--reports=no"}, config.Executable.Args)
	assert.Equal(t, 5*time.Minute, config.Executable.Timeout)
	
	// Verify security configuration
	assert.Equal(t, "public_key", config.Security.AuthMethod)
	assert.Equal(t, "pylint", config.Security.RunAsUser)
	assert.Equal(t, "512MB", config.Security.MaxMemory)
	assert.Equal(t, "1.0", config.Security.MaxCPU)
	
	// Verify input configuration
	assert.Equal(t, []string{"tar.gz", "tar", "zip"}, config.Input.Formats)
	assert.Equal(t, []string{".py", ".pyw"}, config.Input.AllowedExtensions)
	assert.Equal(t, "100MB", config.Input.MaxArchiveSize)
	
	// Verify output configuration
	assert.Equal(t, "json", config.Output.Format)
	assert.Equal(t, "pylint_json", config.Output.Parser)
}

func TestLoadConfig_InvalidPath(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid-config.yaml")
	
	invalidContent := `
service:
  name: "test-service"
  port: invalid-port
    bad-indentation: true
`
	
	err := os.WriteFile(configPath, []byte(invalidContent), 0644)
	require.NoError(t, err)
	
	_, err = LoadConfig(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config")
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Service: ServiceConfig{
					Name: "test-service",
					Port: 8080,
				},
				Executable: ExecutableConfig{
					Path:    "pylint",
					Args:    []string{"--output-format=json"},
					Timeout: 5 * time.Minute,
				},
				Security: SecurityConfig{
					AuthMethod:    "public_key",
					PublicKeyPath: "/tmp/test-public-key.pem", // Will be ignored for validation
					RunAsUser:     "pylint",
					MaxMemory:     "512MB",
					MaxCPU:        "1.0",
				},
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			config: Config{
				Service: ServiceConfig{
					Port: 8080,
				},
			},
			wantErr: true,
			errMsg:  "service name is required",
		},
		{
			name: "invalid port",
			config: Config{
				Service: ServiceConfig{
					Name: "test-service",
					Port: -1,
				},
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "missing executable path",
			config: Config{
				Service: ServiceConfig{
					Name: "test-service",
					Port: 8080,
				},
				Executable: ExecutableConfig{
					Args:    []string{"--output-format=json"},
					Timeout: 5 * time.Minute,
				},
			},
			wantErr: true,
			errMsg:  "executable path is required",
		},
		{
			name: "invalid auth method",
			config: Config{
				Service: ServiceConfig{
					Name: "test-service",
					Port: 8080,
				},
				Executable: ExecutableConfig{
					Path:    "pylint",
					Timeout: 5 * time.Minute,
				},
				Security: SecurityConfig{
					AuthMethod: "invalid",
				},
			},
			wantErr: true,
			errMsg:  "auth_method must be 'public_key'",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(&tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetListenAddr(t *testing.T) {
	config := &Config{
		Service: ServiceConfig{
			Name: "test-service",
			Port: 8080,
		},
	}
	
	assert.Equal(t, ":8080", config.GetListenAddr())
}

func TestConfig_GetTimeoutDuration(t *testing.T) {
	config := &Config{
		Executable: ExecutableConfig{
			Timeout: 5 * time.Minute,
		},
	}
	
	assert.Equal(t, 5*time.Minute, config.GetTimeoutDuration())
}

func TestConfig_IsValidInputFormat(t *testing.T) {
	config := &Config{
		Input: InputConfig{
			Formats: []string{"tar.gz", "tar", "zip"},
		},
	}
	
	assert.True(t, config.IsValidInputFormat("tar.gz"))
	assert.True(t, config.IsValidInputFormat("tar"))
	assert.True(t, config.IsValidInputFormat("zip"))
	assert.False(t, config.IsValidInputFormat("rar"))
	assert.False(t, config.IsValidInputFormat(""))
}

func TestConfig_IsValidFileExtension(t *testing.T) {
	config := &Config{
		Input: InputConfig{
			AllowedExtensions: []string{".py", ".pyw"},
		},
	}
	
	assert.True(t, config.IsValidFileExtension(".py"))
	assert.True(t, config.IsValidFileExtension(".pyw"))
	assert.False(t, config.IsValidFileExtension(".js"))
	assert.False(t, config.IsValidFileExtension(""))
}

// Helper function to create a test public key
func createTestPublicKey(t *testing.T, tempDir string) string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	publicKeyPath := filepath.Join(tempDir, "test-public.pem")
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	return publicKeyPath
}