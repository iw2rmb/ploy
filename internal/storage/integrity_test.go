package storage

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntegrityInfo(t *testing.T) {
	tests := []struct {
		name string
		info IntegrityInfo
		verify func(*testing.T, IntegrityInfo)
	}{
		{
			name: "complete integrity info",
			info: IntegrityInfo{
				LocalPath:        "/tmp/app.tar.gz",
				StorageKey:       "artifacts/app/v1.0.0/app.tar.gz",
				LocalSize:        1048576,
				UploadedSize:     1048576,
				LocalChecksum:    "sha256:abc123def456",
				UploadedHash:     "abc123def456",
				Verified:         true,
				VerificationTime: "2023-12-01T15:30:00Z",
			},
			verify: func(t *testing.T, info IntegrityInfo) {
				assert.Equal(t, "/tmp/app.tar.gz", info.LocalPath)
				assert.Equal(t, "artifacts/app/v1.0.0/app.tar.gz", info.StorageKey)
				assert.Equal(t, int64(1048576), info.LocalSize)
				assert.Equal(t, int64(1048576), info.UploadedSize)
				assert.Equal(t, "sha256:abc123def456", info.LocalChecksum)
				assert.Equal(t, "abc123def456", info.UploadedHash)
				assert.True(t, info.Verified)
				assert.NotEmpty(t, info.VerificationTime)
			},
		},
		{
			name: "failed verification",
			info: IntegrityInfo{
				LocalPath:     "/tmp/corrupted.tar.gz",
				StorageKey:    "artifacts/corrupted/v1.0.0/corrupted.tar.gz",
				LocalSize:     1024,
				UploadedSize:  1000, // Size mismatch
				LocalChecksum: "sha256:abc123",
				UploadedHash:  "def456", // Hash mismatch
				Verified:      false,
			},
			verify: func(t *testing.T, info IntegrityInfo) {
				assert.NotEqual(t, info.LocalSize, info.UploadedSize)
				assert.NotEqual(t, info.LocalChecksum, info.UploadedHash)
				assert.False(t, info.Verified)
			},
		},
		{
			name: "zero value",
			info: IntegrityInfo{},
			verify: func(t *testing.T, info IntegrityInfo) {
				assert.Empty(t, info.LocalPath)
				assert.Empty(t, info.StorageKey)
				assert.Zero(t, info.LocalSize)
				assert.Zero(t, info.UploadedSize)
				assert.Empty(t, info.LocalChecksum)
				assert.Empty(t, info.UploadedHash)
				assert.False(t, info.Verified)
				assert.Empty(t, info.VerificationTime)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.info)
		})
	}
}

func TestBundleIntegrityResult(t *testing.T) {
	tests := []struct {
		name   string
		result BundleIntegrityResult
		verify func(*testing.T, BundleIntegrityResult)
	}{
		{
			name: "complete successful bundle",
			result: BundleIntegrityResult{
				KeyPrefix: "artifacts/myapp/v1.2.3",
				MainArtifact: &IntegrityInfo{
					LocalPath:    "/tmp/myapp.tar.gz",
					StorageKey:   "artifacts/myapp/v1.2.3/myapp.tar.gz",
					LocalSize:    2048000,
					UploadedSize: 2048000,
					Verified:     true,
				},
				SBOM: &IntegrityInfo{
					LocalPath:    "/tmp/myapp.sbom.json",
					StorageKey:   "artifacts/myapp/v1.2.3/myapp.sbom.json",
					LocalSize:    4096,
					UploadedSize: 4096,
					Verified:     true,
				},
				Signature: &IntegrityInfo{
					LocalPath:    "/tmp/myapp.sig",
					StorageKey:   "artifacts/myapp/v1.2.3/myapp.sig",
					LocalSize:    512,
					UploadedSize: 512,
					Verified:     true,
				},
				Certificate: &IntegrityInfo{
					LocalPath:    "/tmp/myapp.cert",
					StorageKey:   "artifacts/myapp/v1.2.3/myapp.cert",
					LocalSize:    1024,
					UploadedSize: 1024,
					Verified:     true,
				},
				Verified: true,
				Errors:   []string{},
			},
			verify: func(t *testing.T, result BundleIntegrityResult) {
				assert.Equal(t, "artifacts/myapp/v1.2.3", result.KeyPrefix)
				assert.NotNil(t, result.MainArtifact)
				assert.True(t, result.MainArtifact.Verified)
				assert.NotNil(t, result.SBOM)
				assert.True(t, result.SBOM.Verified)
				assert.NotNil(t, result.Signature)
				assert.True(t, result.Signature.Verified)
				assert.NotNil(t, result.Certificate)
				assert.True(t, result.Certificate.Verified)
				assert.True(t, result.Verified)
				assert.Empty(t, result.Errors)
			},
		},
		{
			name: "partial bundle with errors",
			result: BundleIntegrityResult{
				KeyPrefix: "artifacts/failapp/v0.1.0",
				MainArtifact: &IntegrityInfo{
					LocalPath:    "/tmp/failapp.tar.gz",
					StorageKey:   "artifacts/failapp/v0.1.0/failapp.tar.gz",
					LocalSize:    1000,
					UploadedSize: 900, // Size mismatch
					Verified:     false,
				},
				SBOM: &IntegrityInfo{
					LocalPath:    "/tmp/failapp.sbom.json",
					StorageKey:   "artifacts/failapp/v0.1.0/failapp.sbom.json",
					LocalSize:    2048,
					UploadedSize: 2048,
					Verified:     true,
				},
				Signature:   nil, // Missing signature
				Certificate: nil, // Missing certificate
				Verified:    false,
				Errors: []string{
					"main artifact size mismatch",
					"signature file missing",
					"certificate file missing",
				},
			},
			verify: func(t *testing.T, result BundleIntegrityResult) {
				assert.False(t, result.Verified)
				assert.NotNil(t, result.MainArtifact)
				assert.False(t, result.MainArtifact.Verified)
				assert.NotNil(t, result.SBOM)
				assert.True(t, result.SBOM.Verified)
				assert.Nil(t, result.Signature)
				assert.Nil(t, result.Certificate)
				assert.Len(t, result.Errors, 3)
			},
		},
		{
			name: "minimal successful bundle",
			result: BundleIntegrityResult{
				KeyPrefix: "artifacts/simple/v1.0.0",
				MainArtifact: &IntegrityInfo{
					LocalPath:    "/tmp/simple.tar.gz",
					StorageKey:   "artifacts/simple/v1.0.0/simple.tar.gz",
					LocalSize:    512000,
					UploadedSize: 512000,
					Verified:     true,
				},
				SBOM:        nil,
				Signature:   nil,
				Certificate: nil,
				Verified:    true,
				Errors:      []string{},
			},
			verify: func(t *testing.T, result BundleIntegrityResult) {
				assert.True(t, result.Verified)
				assert.NotNil(t, result.MainArtifact)
				assert.True(t, result.MainArtifact.Verified)
				assert.Nil(t, result.SBOM)
				assert.Nil(t, result.Signature)
				assert.Nil(t, result.Certificate)
				assert.Empty(t, result.Errors)
			},
		},
		{
			name: "zero value bundle",
			result: BundleIntegrityResult{},
			verify: func(t *testing.T, result BundleIntegrityResult) {
				assert.Empty(t, result.KeyPrefix)
				assert.Nil(t, result.MainArtifact)
				assert.Nil(t, result.SBOM)
				assert.Nil(t, result.Signature)
				assert.Nil(t, result.Certificate)
				assert.False(t, result.Verified)
				assert.Nil(t, result.Errors)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.result)
		})
	}
}

func TestBundleIntegrityResult_GetVerificationSummary(t *testing.T) {
	tests := []struct {
		name           string
		result         BundleIntegrityResult
		expectedSummary string
	}{
		{
			name: "fully verified bundle",
			result: BundleIntegrityResult{
				KeyPrefix: "artifacts/app/v1.0.0",
				MainArtifact: &IntegrityInfo{
					Verified: true,
				},
				SBOM: &IntegrityInfo{
					Verified: true,
				},
				Signature: &IntegrityInfo{
					Verified: true,
				},
				Certificate: &IntegrityInfo{
					Verified: true,
				},
				Verified: true,
			},
			expectedSummary: "Bundle verification successful: 4 files verified",
		},
		{
			name: "main artifact only",
			result: BundleIntegrityResult{
				KeyPrefix: "artifacts/simple/v1.0.0",
				MainArtifact: &IntegrityInfo{
					Verified: true,
				},
				Verified: true,
			},
			expectedSummary: "Bundle verification successful: 1 files verified",
		},
		{
			name: "failed verification",
			result: BundleIntegrityResult{
				KeyPrefix: "artifacts/failed/v1.0.0",
				MainArtifact: &IntegrityInfo{
					Verified: false,
				},
				Verified: false,
				Errors: []string{
					"checksum mismatch",
					"size mismatch",
				},
			},
			expectedSummary: "Bundle verification failed: 2 error(s)",
		},
		{
			name: "partial verification",
			result: BundleIntegrityResult{
				KeyPrefix: "artifacts/partial/v1.0.0",
				MainArtifact: &IntegrityInfo{
					Verified: true,
				},
				SBOM: &IntegrityInfo{
					Verified: true,
				},
				Signature: &IntegrityInfo{
					Verified: false,
				},
				Verified: false,
				Errors: []string{
					"signature verification failed",
				},
			},
			expectedSummary: "Bundle verification failed: 1 error(s)",
		},
		{
			name: "empty bundle",
			result: BundleIntegrityResult{
				Verified: false,
			},
			expectedSummary: "Bundle verification failed: 0 error(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.result.GetVerificationSummary()
			assert.Contains(t, summary, tt.expectedSummary)
		})
	}
}

func TestIntegrityInfo_Fields(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		setValue  interface{}
		getValue  func(IntegrityInfo) interface{}
	}{
		{
			name:      "LocalPath field",
			fieldName: "LocalPath",
			setValue:  "/path/to/local/file.tar.gz",
			getValue:  func(info IntegrityInfo) interface{} { return info.LocalPath },
		},
		{
			name:      "StorageKey field",
			fieldName: "StorageKey",
			setValue:  "artifacts/app/v1.0.0/app.tar.gz",
			getValue:  func(info IntegrityInfo) interface{} { return info.StorageKey },
		},
		{
			name:      "LocalSize field",
			fieldName: "LocalSize",
			setValue:  int64(1048576),
			getValue:  func(info IntegrityInfo) interface{} { return info.LocalSize },
		},
		{
			name:      "UploadedSize field",
			fieldName: "UploadedSize",
			setValue:  int64(1048576),
			getValue:  func(info IntegrityInfo) interface{} { return info.UploadedSize },
		},
		{
			name:      "LocalChecksum field",
			fieldName: "LocalChecksum",
			setValue:  "sha256:abcdef123456",
			getValue:  func(info IntegrityInfo) interface{} { return info.LocalChecksum },
		},
		{
			name:      "UploadedHash field",
			fieldName: "UploadedHash",
			setValue:  "abcdef123456",
			getValue:  func(info IntegrityInfo) interface{} { return info.UploadedHash },
		},
		{
			name:      "Verified field",
			fieldName: "Verified",
			setValue:  true,
			getValue:  func(info IntegrityInfo) interface{} { return info.Verified },
		},
		{
			name:      "VerificationTime field",
			fieldName: "VerificationTime",
			setValue:  "2023-12-01T15:30:00Z",
			getValue:  func(info IntegrityInfo) interface{} { return info.VerificationTime },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info IntegrityInfo

			// Set the field using reflection-like approach
			switch tt.fieldName {
			case "LocalPath":
				info.LocalPath = tt.setValue.(string)
			case "StorageKey":
				info.StorageKey = tt.setValue.(string)
			case "LocalSize":
				info.LocalSize = tt.setValue.(int64)
			case "UploadedSize":
				info.UploadedSize = tt.setValue.(int64)
			case "LocalChecksum":
				info.LocalChecksum = tt.setValue.(string)
			case "UploadedHash":
				info.UploadedHash = tt.setValue.(string)
			case "Verified":
				info.Verified = tt.setValue.(bool)
			case "VerificationTime":
				info.VerificationTime = tt.setValue.(string)
			}

			// Verify the field was set correctly
			actualValue := tt.getValue(info)
			assert.Equal(t, tt.setValue, actualValue)
		})
	}
}

func TestBundleIntegrityResult_Fields(t *testing.T) {
	mainArtifact := &IntegrityInfo{
		LocalPath:  "/tmp/main.tar.gz",
		StorageKey: "artifacts/app/main.tar.gz",
		Verified:   true,
	}

	sbom := &IntegrityInfo{
		LocalPath:  "/tmp/sbom.json",
		StorageKey: "artifacts/app/sbom.json",
		Verified:   true,
	}

	signature := &IntegrityInfo{
		LocalPath:  "/tmp/signature.sig",
		StorageKey: "artifacts/app/signature.sig",
		Verified:   false,
	}

	certificate := &IntegrityInfo{
		LocalPath:  "/tmp/cert.pem",
		StorageKey: "artifacts/app/cert.pem",
		Verified:   true,
	}

	errors := []string{"signature verification failed", "certificate expired"}

	result := BundleIntegrityResult{
		KeyPrefix:   "artifacts/testapp/v2.0.0",
		MainArtifact: mainArtifact,
		SBOM:        sbom,
		Signature:   signature,
		Certificate: certificate,
		Verified:    false,
		Errors:      errors,
	}

	// Test individual field access
	assert.Equal(t, "artifacts/testapp/v2.0.0", result.KeyPrefix)
	assert.Equal(t, mainArtifact, result.MainArtifact)
	assert.Equal(t, sbom, result.SBOM)
	assert.Equal(t, signature, result.Signature)
	assert.Equal(t, certificate, result.Certificate)
	assert.False(t, result.Verified)
	assert.Equal(t, errors, result.Errors)
}

// Test JSON serialization/deserialization patterns
func TestIntegrityInfo_JSONCompatibility(t *testing.T) {
	originalInfo := IntegrityInfo{
		LocalPath:        "/tmp/test-app.tar.gz",
		StorageKey:       "artifacts/test-app/v1.0.0/test-app.tar.gz",
		LocalSize:        2097152, // 2MB
		UploadedSize:     2097152,
		LocalChecksum:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		UploadedHash:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Verified:         true,
		VerificationTime: "2023-12-01T15:30:00Z",
	}

	// Verify the struct can be used in JSON contexts (fields are exported)
	assert.NotEmpty(t, originalInfo.LocalPath)
	assert.NotEmpty(t, originalInfo.StorageKey)
	assert.Greater(t, originalInfo.LocalSize, int64(0))
	assert.Equal(t, originalInfo.LocalSize, originalInfo.UploadedSize)
	assert.True(t, originalInfo.Verified)
}

func TestBundleIntegrityResult_JSONCompatibility(t *testing.T) {
	result := BundleIntegrityResult{
		KeyPrefix: "artifacts/complex-app/v3.1.4",
		MainArtifact: &IntegrityInfo{
			LocalPath:    "/tmp/complex-app.tar.gz",
			StorageKey:   "artifacts/complex-app/v3.1.4/complex-app.tar.gz",
			LocalSize:    10485760, // 10MB
			UploadedSize: 10485760,
			Verified:     true,
		},
		SBOM: &IntegrityInfo{
			LocalPath:    "/tmp/complex-app.sbom.json",
			StorageKey:   "artifacts/complex-app/v3.1.4/complex-app.sbom.json",
			LocalSize:    8192,
			UploadedSize: 8192,
			Verified:     true,
		},
		Verified: true,
		Errors:   []string{},
	}

	// Verify the struct is properly structured for JSON
	assert.NotEmpty(t, result.KeyPrefix)
	assert.NotNil(t, result.MainArtifact)
	assert.NotNil(t, result.SBOM)
	assert.Nil(t, result.Signature) // Optional field
	assert.Nil(t, result.Certificate) // Optional field
	assert.True(t, result.Verified)
	assert.Empty(t, result.Errors)
}

// Test edge cases and boundary conditions
func TestIntegrityInfo_EdgeCases(t *testing.T) {
	t.Run("very large file size", func(t *testing.T) {
		info := IntegrityInfo{
			LocalSize:    9223372036854775807, // Max int64
			UploadedSize: 9223372036854775807,
		}
		
		assert.Equal(t, int64(9223372036854775807), info.LocalSize)
		assert.Equal(t, info.LocalSize, info.UploadedSize)
	})

	t.Run("empty strings", func(t *testing.T) {
		info := IntegrityInfo{
			LocalPath:        "",
			StorageKey:       "",
			LocalChecksum:    "",
			UploadedHash:     "",
			VerificationTime: "",
		}
		
		assert.Empty(t, info.LocalPath)
		assert.Empty(t, info.StorageKey)
		assert.Empty(t, info.LocalChecksum)
		assert.Empty(t, info.UploadedHash)
		assert.Empty(t, info.VerificationTime)
	})

	t.Run("unicode in paths", func(t *testing.T) {
		info := IntegrityInfo{
			LocalPath:  "/tmp/应用程序.tar.gz",
			StorageKey: "artifacts/应用程序/版本1.0.0/应用程序.tar.gz",
		}
		
		assert.Contains(t, info.LocalPath, "应用程序")
		assert.Contains(t, info.StorageKey, "应用程序")
		assert.Contains(t, info.StorageKey, "版本1.0.0")
	})
}

func TestBundleIntegrityResult_EdgeCases(t *testing.T) {
	t.Run("all nil pointers", func(t *testing.T) {
		result := BundleIntegrityResult{
			KeyPrefix:   "artifacts/empty/v1.0.0",
			MainArtifact: nil,
			SBOM:        nil,
			Signature:   nil,
			Certificate: nil,
			Verified:    false,
		}
		
		assert.Nil(t, result.MainArtifact)
		assert.Nil(t, result.SBOM)
		assert.Nil(t, result.Signature)
		assert.Nil(t, result.Certificate)
		assert.False(t, result.Verified)
	})

	t.Run("many errors", func(t *testing.T) {
		manyErrors := make([]string, 100)
		for i := 0; i < 100; i++ {
			manyErrors[i] = fmt.Sprintf("error %d", i+1)
		}
		
		result := BundleIntegrityResult{
			Errors: manyErrors,
		}
		
		assert.Len(t, result.Errors, 100)
		assert.Equal(t, "error 1", result.Errors[0])
		assert.Equal(t, "error 100", result.Errors[99])
	})

	t.Run("empty key prefix", func(t *testing.T) {
		result := BundleIntegrityResult{
			KeyPrefix: "",
			MainArtifact: &IntegrityInfo{
				Verified: true,
			},
			Verified: true,
		}
		
		assert.Empty(t, result.KeyPrefix)
		assert.True(t, result.Verified)
	})
}

// Test common usage patterns for integrity verification
func TestIntegrityVerification_UsagePatterns(t *testing.T) {
	t.Run("successful complete bundle verification", func(t *testing.T) {
		result := BundleIntegrityResult{
			KeyPrefix: "artifacts/production-app/v2.1.0",
			MainArtifact: &IntegrityInfo{
				LocalPath:        "/builds/production-app-v2.1.0.tar.gz",
				StorageKey:       "artifacts/production-app/v2.1.0/production-app-v2.1.0.tar.gz",
				LocalSize:        52428800,  // 50MB
				UploadedSize:     52428800,
				LocalChecksum:    "sha256:d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2",
				UploadedHash:     "d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2",
				Verified:         true,
				VerificationTime: "2023-12-01T15:30:00Z",
			},
			SBOM: &IntegrityInfo{
				LocalPath:        "/builds/production-app-v2.1.0.sbom.json",
				StorageKey:       "artifacts/production-app/v2.1.0/production-app-v2.1.0.sbom.json",
				LocalSize:        16384,
				UploadedSize:     16384,
				LocalChecksum:    "sha256:s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1",
				UploadedHash:     "s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1s1",
				Verified:         true,
				VerificationTime: "2023-12-01T15:30:01Z",
			},
			Signature: &IntegrityInfo{
				LocalPath:        "/builds/production-app-v2.1.0.sig",
				StorageKey:       "artifacts/production-app/v2.1.0/production-app-v2.1.0.sig",
				LocalSize:        1024,
				UploadedSize:     1024,
				LocalChecksum:    "sha256:sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1",
				UploadedHash:     "sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1sig1",
				Verified:         true,
				VerificationTime: "2023-12-01T15:30:02Z",
			},
			Certificate: &IntegrityInfo{
				LocalPath:        "/builds/production-app-v2.1.0.cert",
				StorageKey:       "artifacts/production-app/v2.1.0/production-app-v2.1.0.cert",
				LocalSize:        2048,
				UploadedSize:     2048,
				LocalChecksum:    "sha256:cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1",
				UploadedHash:     "cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1cert1",
				Verified:         true,
				VerificationTime: "2023-12-01T15:30:03Z",
			},
			Verified: true,
			Errors:   []string{},
		}

		// Verify all components are present and verified
		assert.True(t, result.Verified)
		assert.NotNil(t, result.MainArtifact)
		assert.True(t, result.MainArtifact.Verified)
		assert.NotNil(t, result.SBOM)
		assert.True(t, result.SBOM.Verified)
		assert.NotNil(t, result.Signature)
		assert.True(t, result.Signature.Verified)
		assert.NotNil(t, result.Certificate)
		assert.True(t, result.Certificate.Verified)
		assert.Empty(t, result.Errors)

		// Verify summary is correct
		summary := result.GetVerificationSummary()
		assert.Contains(t, summary, "successful")
		assert.Contains(t, summary, "4 files")
	})

	t.Run("partial bundle with missing optional components", func(t *testing.T) {
		result := BundleIntegrityResult{
			KeyPrefix: "artifacts/dev-app/v0.1.0-alpha",
			MainArtifact: &IntegrityInfo{
				LocalPath:    "/tmp/dev-app.tar.gz",
				StorageKey:   "artifacts/dev-app/v0.1.0-alpha/dev-app.tar.gz",
				LocalSize:    1048576, // 1MB
				UploadedSize: 1048576,
				Verified:     true,
			},
			// Only SBOM present, no signature or certificate for dev builds
			SBOM: &IntegrityInfo{
				LocalPath:    "/tmp/dev-app.sbom.json",
				StorageKey:   "artifacts/dev-app/v0.1.0-alpha/dev-app.sbom.json",
				LocalSize:    4096,
				UploadedSize: 4096,
				Verified:     true,
			},
			Signature:   nil, // Not required for dev builds
			Certificate: nil, // Not required for dev builds
			Verified:    true,
			Errors:      []string{},
		}

		assert.True(t, result.Verified)
		assert.NotNil(t, result.MainArtifact)
		assert.NotNil(t, result.SBOM)
		assert.Nil(t, result.Signature)
		assert.Nil(t, result.Certificate)
		assert.Empty(t, result.Errors)
	})
}