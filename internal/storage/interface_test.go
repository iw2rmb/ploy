package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPutObjectResult(t *testing.T) {
	result := &PutObjectResult{
		ETag:     "test-etag-123",
		Location: "https://storage.example.com/bucket/key",
		Size:     1024,
	}

	assert.Equal(t, "test-etag-123", result.ETag)
	assert.Equal(t, "https://storage.example.com/bucket/key", result.Location)
	assert.Equal(t, int64(1024), result.Size)
}

func TestObjectInfo(t *testing.T) {
	objInfo := &ObjectInfo{
		Key:          "test/object/key",
		Size:         2048,
		LastModified: "2023-12-01T10:00:00Z",
		ETag:         "test-etag-456",
		ContentType:  "application/octet-stream",
	}

	assert.Equal(t, "test/object/key", objInfo.Key)
	assert.Equal(t, int64(2048), objInfo.Size)
	assert.Equal(t, "2023-12-01T10:00:00Z", objInfo.LastModified)
	assert.Equal(t, "test-etag-456", objInfo.ETag)
	assert.Equal(t, "application/octet-stream", objInfo.ContentType)
}

func TestObjectInfo_Fields(t *testing.T) {
	tests := []struct {
		name        string
		objectInfo  ObjectInfo
		fieldName   string
		fieldValue  interface{}
	}{
		{
			name: "key field",
			objectInfo: ObjectInfo{
				Key: "artifacts/app/v1.0.0/app.tar.gz",
			},
			fieldName:  "Key",
			fieldValue: "artifacts/app/v1.0.0/app.tar.gz",
		},
		{
			name: "size field",
			objectInfo: ObjectInfo{
				Size: 1048576, // 1MB
			},
			fieldName:  "Size",
			fieldValue: int64(1048576),
		},
		{
			name: "content type field",
			objectInfo: ObjectInfo{
				ContentType: "application/gzip",
			},
			fieldName:  "ContentType",
			fieldValue: "application/gzip",
		},
		{
			name: "etag field",
			objectInfo: ObjectInfo{
				ETag: "d41d8cd98f00b204e9800998ecf8427e",
			},
			fieldName:  "ETag",
			fieldValue: "d41d8cd98f00b204e9800998ecf8427e",
		},
		{
			name: "last modified field",
			objectInfo: ObjectInfo{
				LastModified: "2023-12-01T15:30:00Z",
			},
			fieldName:  "LastModified",
			fieldValue: "2023-12-01T15:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.fieldName {
			case "Key":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.Key)
			case "Size":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.Size)
			case "ContentType":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.ContentType)
			case "ETag":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.ETag)
			case "LastModified":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.LastModified)
			}
		})
	}
}

func TestPutObjectResult_Fields(t *testing.T) {
	tests := []struct {
		name       string
		result     PutObjectResult
		fieldName  string
		fieldValue interface{}
	}{
		{
			name: "etag field",
			result: PutObjectResult{
				ETag: "\"33a64df551425fcc55e4d42a148795d9f25f89d4\"",
			},
			fieldName:  "ETag",
			fieldValue: "\"33a64df551425fcc55e4d42a148795d9f25f89d4\"",
		},
		{
			name: "location field",
			result: PutObjectResult{
				Location: "https://seaweedfs.local:8888/bucket/path/to/object",
			},
			fieldName:  "Location",
			fieldValue: "https://seaweedfs.local:8888/bucket/path/to/object",
		},
		{
			name: "size field",
			result: PutObjectResult{
				Size: 5242880, // 5MB
			},
			fieldName:  "Size",
			fieldValue: int64(5242880),
		},
		{
			name: "zero size",
			result: PutObjectResult{
				Size: 0,
			},
			fieldName:  "Size",
			fieldValue: int64(0),
		},
		{
			name: "empty etag",
			result: PutObjectResult{
				ETag: "",
			},
			fieldName:  "ETag",
			fieldValue: "",
		},
		{
			name: "empty location",
			result: PutObjectResult{
				Location: "",
			},
			fieldName:  "Location",
			fieldValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.fieldName {
			case "ETag":
				assert.Equal(t, tt.fieldValue, tt.result.ETag)
			case "Location":
				assert.Equal(t, tt.fieldValue, tt.result.Location)
			case "Size":
				assert.Equal(t, tt.fieldValue, tt.result.Size)
			}
		})
	}
}

// Test struct initialization patterns
func TestObjectInfo_Initialization(t *testing.T) {
	tests := []struct {
		name   string
		init   func() ObjectInfo
		verify func(*testing.T, ObjectInfo)
	}{
		{
			name: "zero value initialization",
			init: func() ObjectInfo {
				return ObjectInfo{}
			},
			verify: func(t *testing.T, obj ObjectInfo) {
				assert.Empty(t, obj.Key)
				assert.Zero(t, obj.Size)
				assert.Empty(t, obj.LastModified)
				assert.Empty(t, obj.ETag)
				assert.Empty(t, obj.ContentType)
			},
		},
		{
			name: "partial initialization",
			init: func() ObjectInfo {
				return ObjectInfo{
					Key:  "partial/key",
					Size: 1024,
				}
			},
			verify: func(t *testing.T, obj ObjectInfo) {
				assert.Equal(t, "partial/key", obj.Key)
				assert.Equal(t, int64(1024), obj.Size)
				assert.Empty(t, obj.LastModified)
				assert.Empty(t, obj.ETag)
				assert.Empty(t, obj.ContentType)
			},
		},
		{
			name: "full initialization",
			init: func() ObjectInfo {
				return ObjectInfo{
					Key:          "full/object/key",
					Size:         4096,
					LastModified: "2023-12-01T12:00:00Z",
					ETag:         "full-etag",
					ContentType:  "application/json",
				}
			},
			verify: func(t *testing.T, obj ObjectInfo) {
				assert.Equal(t, "full/object/key", obj.Key)
				assert.Equal(t, int64(4096), obj.Size)
				assert.Equal(t, "2023-12-01T12:00:00Z", obj.LastModified)
				assert.Equal(t, "full-etag", obj.ETag)
				assert.Equal(t, "application/json", obj.ContentType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.init()
			tt.verify(t, obj)
		})
	}
}

func TestPutObjectResult_Initialization(t *testing.T) {
	tests := []struct {
		name   string
		init   func() PutObjectResult
		verify func(*testing.T, PutObjectResult)
	}{
		{
			name: "zero value initialization",
			init: func() PutObjectResult {
				return PutObjectResult{}
			},
			verify: func(t *testing.T, result PutObjectResult) {
				assert.Empty(t, result.ETag)
				assert.Empty(t, result.Location)
				assert.Zero(t, result.Size)
			},
		},
		{
			name: "typical success result",
			init: func() PutObjectResult {
				return PutObjectResult{
					ETag:     "\"abc123def456\"",
					Location: "https://storage.local/bucket/key",
					Size:     2048,
				}
			},
			verify: func(t *testing.T, result PutObjectResult) {
				assert.Equal(t, "\"abc123def456\"", result.ETag)
				assert.Equal(t, "https://storage.local/bucket/key", result.Location)
				assert.Equal(t, int64(2048), result.Size)
			},
		},
		{
			name: "large file result",
			init: func() PutObjectResult {
				return PutObjectResult{
					ETag:     "\"large-file-etag\"",
					Location: "https://storage.remote/artifacts/large-app.tar.gz",
					Size:     1073741824, // 1GB
				}
			},
			verify: func(t *testing.T, result PutObjectResult) {
				assert.Equal(t, "\"large-file-etag\"", result.ETag)
				assert.Equal(t, "https://storage.remote/artifacts/large-app.tar.gz", result.Location)
				assert.Equal(t, int64(1073741824), result.Size)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.init()
			tt.verify(t, result)
		})
	}
}

// Test common usage patterns
func TestObjectInfo_CommonPatterns(t *testing.T) {
	t.Run("artifact object pattern", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "artifacts/myapp/v1.2.3/myapp-v1.2.3.tar.gz",
			Size:         10485760, // 10MB
			LastModified: "2023-12-01T14:30:00Z",
			ETag:         "\"artifact-etag-12345\"",
			ContentType:  "application/gzip",
		}

		assert.Contains(t, obj.Key, "artifacts/")
		assert.Contains(t, obj.Key, "/v1.2.3/")
		assert.Greater(t, obj.Size, int64(0))
		assert.NotEmpty(t, obj.ETag)
		assert.Equal(t, "application/gzip", obj.ContentType)
	})

	t.Run("sbom object pattern", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "artifacts/myapp/v1.2.3/myapp-v1.2.3.sbom.json",
			Size:         2048,
			LastModified: "2023-12-01T14:30:00Z",
			ETag:         "\"sbom-etag-67890\"",
			ContentType:  "application/json",
		}

		assert.Contains(t, obj.Key, ".sbom.json")
		assert.Equal(t, "application/json", obj.ContentType)
		assert.Greater(t, obj.Size, int64(0))
	})

	t.Run("signature object pattern", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "artifacts/myapp/v1.2.3/myapp-v1.2.3.sig",
			Size:         512,
			LastModified: "2023-12-01T14:30:00Z",
			ETag:         "\"sig-etag-abcde\"",
			ContentType:  "application/octet-stream",
		}

		assert.Contains(t, obj.Key, ".sig")
		assert.Greater(t, obj.Size, int64(0))
		assert.Equal(t, "application/octet-stream", obj.ContentType)
	})
}

func TestPutObjectResult_CommonPatterns(t *testing.T) {
	t.Run("small file upload result", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"small-file-etag\"",
			Location: "https://seaweedfs.local:8888/artifacts/config.json",
			Size:     256,
		}

		assert.NotEmpty(t, result.ETag)
		assert.Contains(t, result.Location, "artifacts/")
		assert.Less(t, result.Size, int64(1024)) // Small file
	})

	t.Run("large artifact upload result", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"large-artifact-etag\"",
			Location: "https://seaweedfs.local:8888/artifacts/app/v2.0.0/app-v2.0.0.tar.gz",
			Size:     104857600, // 100MB
		}

		assert.NotEmpty(t, result.ETag)
		assert.Contains(t, result.Location, "artifacts/")
		assert.Contains(t, result.Location, "v2.0.0")
		assert.Greater(t, result.Size, int64(50*1024*1024)) // Large file
	})

	t.Run("secure artifact with etag quotes", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"quoted-etag-with-special-chars\"",
			Location: "https://secure.storage.com/bucket/secure-app.tar.gz",
			Size:     5242880, // 5MB
		}

		assert.HasPrefix(t, result.ETag, "\"")
		assert.HasSuffix(t, result.ETag, "\"")
		assert.Contains(t, result.Location, "https://")
	})
}

// Test edge cases and boundary conditions
func TestObjectInfo_EdgeCases(t *testing.T) {
	t.Run("very large size", func(t *testing.T) {
		obj := ObjectInfo{
			Key:  "large-file",
			Size: 9223372036854775807, // Max int64
		}

		assert.Equal(t, int64(9223372036854775807), obj.Size)
		assert.Greater(t, obj.Size, int64(0))
	})

	t.Run("empty key with data", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "",
			Size:         1024,
			LastModified: "2023-12-01T10:00:00Z",
		}

		assert.Empty(t, obj.Key)
		assert.Greater(t, obj.Size, int64(0))
		assert.NotEmpty(t, obj.LastModified)
	})

	t.Run("unicode in key", func(t *testing.T) {
		obj := ObjectInfo{
			Key:  "artifacts/应用程序/版本1.0.0/app.tar.gz",
			Size: 1024,
		}

		assert.Contains(t, obj.Key, "应用程序")
		assert.Contains(t, obj.Key, "版本1.0.0")
	})

	t.Run("special characters in etag", func(t *testing.T) {
		obj := ObjectInfo{
			ETag: "\"etag-with-dashes_and_underscores.and.dots\"",
		}

		assert.Contains(t, obj.ETag, "-")
		assert.Contains(t, obj.ETag, "_")
		assert.Contains(t, obj.ETag, ".")
	})
}

func TestPutObjectResult_EdgeCases(t *testing.T) {
	t.Run("zero size file", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"empty-file-etag\"",
			Location: "https://storage.com/empty-file",
			Size:     0,
		}

		assert.Equal(t, int64(0), result.Size)
		assert.NotEmpty(t, result.ETag)
		assert.NotEmpty(t, result.Location)
	})

	t.Run("very long location URL", func(t *testing.T) {
		longURL := "https://very-long-storage-domain-name.example.com:8888/very/deep/nested/directory/structure/with/many/levels/artifacts/app/version/1.0.0/app-artifact.tar.gz"
		result := PutObjectResult{
			ETag:     "\"long-url-etag\"",
			Location: longURL,
			Size:     1024,
		}

		assert.Equal(t, longURL, result.Location)
		assert.Greater(t, len(result.Location), 100)
	})

	t.Run("etag without quotes", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "unquoted-etag-value",
			Location: "https://storage.com/file",
			Size:     512,
		}

		assert.Equal(t, "unquoted-etag-value", result.ETag)
		assert.NotContains(t, result.ETag, "\"")
	})
}