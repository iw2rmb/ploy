package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for GetImageSize function
func TestGetImageSize(t *testing.T) {
	tmpDir := createTestTempDir(t)
	defer cleanupTestTempDir(t, tmpDir)

	tests := []struct {
		name           string
		imagePath      string
		dockerImage    string
		lane           string
		setup          func() string // Returns actual path to use
		expectError    bool
		expectedFields map[string]interface{}
	}{
		{
			name: "measure file size",
			setup: func() string {
				testFile := filepath.Join(tmpDir, "test_artifact.bin")
				content := make([]byte, 1024*1024) // 1MB
				for i := range content {
					content[i] = byte(i % 256)
				}
				err := os.WriteFile(testFile, content, 0644)
				require.NoError(t, err)
				return testFile
			},
			lane:        "A",
			expectError: false,
			expectedFields: map[string]interface{}{
				"measurement_type": "file",
				"lane":             "A",
				"size_mb":          float64(1.0), // Approximately 1MB
			},
		},
		{
			name: "measure small file size",
			setup: func() string {
				testFile := filepath.Join(tmpDir, "small_artifact.bin")
				content := make([]byte, 512) // 512 bytes
				err := os.WriteFile(testFile, content, 0644)
				require.NoError(t, err)
				return testFile
			},
			lane:        "B",
			expectError: false,
			expectedFields: map[string]interface{}{
				"measurement_type": "file",
				"lane":             "B",
			},
		},
		{
			name:        "non-existent file",
			imagePath:   "/path/to/nonexistent/file.bin",
			lane:        "A",
			expectError: true,
		},
		{
			name:        "no artifact specified",
			imagePath:   "",
			dockerImage: "",
			lane:        "A",
			expectError: true,
		},
		{
			name:        "docker image measurement (mocked)",
			dockerImage: "test:latest",
			lane:        "E",
			expectError: true, // Will fail in test environment without Docker
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var imagePath, dockerImage string

			if tt.setup != nil {
				imagePath = tt.setup()
			} else {
				imagePath = tt.imagePath
				dockerImage = tt.dockerImage
			}

			result, err := GetImageSize(imagePath, dockerImage, tt.lane)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				// Verify expected fields
				if measurementType, ok := tt.expectedFields["measurement_type"]; ok {
					assert.Equal(t, measurementType, result.MeasurementType)
				}
				if lane, ok := tt.expectedFields["lane"]; ok {
					assert.Equal(t, lane, result.Lane)
				}
				if sizeMB, ok := tt.expectedFields["size_mb"]; ok {
					assert.InDelta(t, sizeMB, result.SizeMB, 0.1) // Allow small delta for MB calculation
				}

				// Common assertions
				assert.True(t, result.SizeBytes > 0, "Size bytes should be positive")
				assert.True(t, result.SizeMB >= 0, "Size MB should be non-negative")
				assert.Equal(t, tt.lane, result.Lane)
			}
		})
	}
}

// Tests for getFileSize function
func TestGetFileSize(t *testing.T) {
	tmpDir := createTestTempDir(t)
	defer cleanupTestTempDir(t, tmpDir)

	tests := []struct {
		name        string
		setup       func() (string, int64) // Returns file path and expected size
		lane        string
		expectError bool
	}{
		{
			name: "regular file",
			setup: func() (string, int64) {
				content := []byte("test content for file size measurement")
				testFile := filepath.Join(tmpDir, "regular_file.txt")
				err := os.WriteFile(testFile, content, 0644)
				require.NoError(t, err)
				return testFile, int64(len(content))
			},
			lane:        "A",
			expectError: false,
		},
		{
			name: "empty file",
			setup: func() (string, int64) {
				testFile := filepath.Join(tmpDir, "empty_file.txt")
				err := os.WriteFile(testFile, []byte{}, 0644)
				require.NoError(t, err)
				return testFile, 0
			},
			lane:        "B",
			expectError: false,
		},
		{
			name: "large file",
			setup: func() (string, int64) {
				content := make([]byte, 2*1024*1024) // 2MB
				for i := range content {
					content[i] = byte(i % 256)
				}
				testFile := filepath.Join(tmpDir, "large_file.bin")
				err := os.WriteFile(testFile, content, 0644)
				require.NoError(t, err)
				return testFile, int64(len(content))
			},
			lane:        "C",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath, expectedSize := tt.setup()

			result, err := getFileSize(filePath, tt.lane)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.Equal(t, filePath, result.FilePath)
				assert.Equal(t, expectedSize, result.SizeBytes)
				assert.Equal(t, float64(expectedSize)/(1024*1024), result.SizeMB)
				assert.Equal(t, tt.lane, result.Lane)
				assert.Equal(t, "file", result.MeasurementType)
				assert.Empty(t, result.DockerImage)
			}
		})
	}
}

// Tests for parseDockerSize function
func TestParseDockerSize(t *testing.T) {
	tests := []struct {
		name        string
		sizeStr     string
		expected    int64
		expectError bool
	}{
		// Bytes
		{"bytes format", "1024B", 1024, false},
		{"bytes without unit", "1024", 1024, false},
		{"zero bytes", "0", 0, false},
		{"zero bytes with unit", "0B", 0, false},

		// Kilobytes
		{"kilobytes format", "512KB", 512 * 1024, false},
		{"kilobytes decimal", "1.5KB", int64(1.5 * 1024), false},
		{"kilobytes lowercase", "256kb", 256 * 1024, false},

		// Megabytes
		{"megabytes format", "100MB", 100 * 1024 * 1024, false},
		{"megabytes decimal", "2.5MB", int64(2.5 * 1024 * 1024), false},
		{"megabytes lowercase", "50mb", 50 * 1024 * 1024, false},

		// Gigabytes
		{"gigabytes format", "2GB", 2 * 1024 * 1024 * 1024, false},
		{"gigabytes decimal", "1.5GB", int64(1.5 * 1024 * 1024 * 1024), false},
		{"gigabytes lowercase", "3gb", 3 * 1024 * 1024 * 1024, false},

		// Edge cases and errors
		{"empty string", "", 0, true},
		{"invalid format", "invalid", 0, true},
		{"negative number", "-100MB", 0, true},
		{"invalid unit", "100XB", 0, true},
		{"no number", "MB", 0, true},
		{"multiple units", "100MBKB", 0, true},

		// Whitespace handling
		{"leading whitespace", "  100MB", 100 * 1024 * 1024, false},
		{"trailing whitespace", "100MB  ", 100 * 1024 * 1024, false},
		{"both whitespace", "  100MB  ", 100 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDockerSize(tt.sizeStr)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, int64(0), result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// Tests for GetLaneSizeLimits function
func TestGetLaneSizeLimits(t *testing.T) {
	limits := GetLaneSizeLimits()

	// Verify we have limits for all expected lanes
	expectedLanes := []string{"A", "B", "C", "D", "E", "F"}
	assert.Len(t, limits, len(expectedLanes))

	// Verify each lane has proper configuration
	laneMap := make(map[string]LaneSizeLimit)
	for _, limit := range limits {
		laneMap[limit.Lane] = limit
	}

	for _, expectedLane := range expectedLanes {
		t.Run("lane_"+expectedLane, func(t *testing.T) {
			limit, exists := laneMap[expectedLane]
			assert.True(t, exists, "Lane %s should have size limit defined", expectedLane)
			assert.Equal(t, expectedLane, limit.Lane)
			assert.Greater(t, limit.MaxSizeMB, int64(0), "Max size should be positive")
			assert.NotEmpty(t, limit.Description, "Description should not be empty")
		})
	}

	// Verify ordering makes sense (smaller lanes should have smaller limits)
	assert.True(t, laneMap["A"].MaxSizeMB < laneMap["B"].MaxSizeMB, "Lane A should be smaller than B")
	assert.True(t, laneMap["B"].MaxSizeMB < laneMap["C"].MaxSizeMB, "Lane B should be smaller than C")
	assert.True(t, laneMap["D"].MaxSizeMB < laneMap["E"].MaxSizeMB, "Lane D should be smaller than E")
	assert.True(t, laneMap["E"].MaxSizeMB < laneMap["F"].MaxSizeMB, "Lane E should be smaller than F")
}

// Tests for GetLaneSizeLimit function
func TestGetLaneSizeLimit(t *testing.T) {
	tests := []struct {
		name         string
		lane         string
		expectError  bool
		expectedLane string
	}{
		{"valid lane A", "A", false, "A"},
		{"valid lane B", "B", false, "B"},
		{"valid lane C", "C", false, "C"},
		{"valid lane D", "D", false, "D"},
		{"valid lane E", "E", false, "E"},
		{"valid lane F", "F", false, "F"},
		{"lowercase lane", "a", false, "A"}, // Should be case-insensitive
		{"lowercase lane e", "e", false, "E"},
		{"invalid lane", "Z", true, ""},
		{"empty lane", "", true, ""},
		{"numeric lane", "1", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetLaneSizeLimit(tt.lane)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result.Lane)
				assert.Contains(t, err.Error(), "no size limit defined")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLane, result.Lane)
				assert.Greater(t, result.MaxSizeMB, int64(0))
				assert.NotEmpty(t, result.Description)
			}
		})
	}
}

// Tests for FormatSize function
func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		// Bytes
		{"zero bytes", 0, "0 B"},
		{"single byte", 1, "1 B"},
		{"small bytes", 512, "512 B"},
		{"max bytes before KB", 1023, "1023 B"},

		// Kilobytes
		{"exactly 1 KB", 1024, "1.0 KB"},
		{"fractional KB", 1536, "1.5 KB"},
		{"large KB", 512 * 1024, "512.0 KB"},

		// Megabytes
		{"exactly 1 MB", 1024 * 1024, "1.0 MB"},
		{"fractional MB", int64(2.5 * 1024 * 1024), "2.5 MB"},
		{"large MB", 512 * 1024 * 1024, "512.0 MB"},

		// Gigabytes
		{"exactly 1 GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"fractional GB", int64(1.5 * 1024 * 1024 * 1024), "1.5 GB"},
		{"large GB", 10 * 1024 * 1024 * 1024, "10.0 GB"},

		// Terabytes
		{"exactly 1 TB", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"fractional TB", int64(2.5 * 1024 * 1024 * 1024 * 1024), "2.5 TB"},

		// Petabytes
		{"exactly 1 PB", 1024 * 1024 * 1024 * 1024 * 1024, "1.0 PB"},

		// Exabytes (edge case)
		{"exactly 1 EB", 1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1.0 EB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for getDockerImageSize function (requires Docker - mostly error cases)
func TestGetDockerImageSize(t *testing.T) {
	tests := []struct {
		name        string
		dockerImage string
		lane        string
		expectError bool
		skipTest    bool // Skip if Docker not available
	}{
		{
			name:        "non-existent docker image",
			dockerImage: "nonexistent:image",
			lane:        "E",
			expectError: true,
			skipTest:    false, // Always test error case
		},
		{
			name:        "empty docker image name",
			dockerImage: "",
			lane:        "E",
			expectError: true,
			skipTest:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipTest {
				// Check if Docker is available
				if _, err := exec.LookPath("docker"); err != nil {
					t.Skip("Skipping test - Docker not available")
					return
				}
			}

			result, err := getDockerImageSize(tt.dockerImage, tt.lane)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.dockerImage, result.DockerImage)
				assert.Equal(t, tt.lane, result.Lane)
				assert.Equal(t, "docker", result.MeasurementType)
			}
		})
	}
}

// Tests for getDetailedDockerSize function
func TestGetDetailedDockerSize(t *testing.T) {
	tests := []struct {
		name        string
		dockerImage string
		skipTest    bool
	}{
		{
			name:        "non-existent docker image",
			dockerImage: "nonexistent:image",
			skipTest:    false, // Test error handling
		},
		{
			name:        "empty image name",
			dockerImage: "",
			skipTest:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipTest {
				if _, err := exec.LookPath("docker"); err != nil {
					t.Skip("Skipping test - Docker not available")
					return
				}
			}

			compressedSize, uncompressedSize := getDetailedDockerSize(tt.dockerImage)

			// For non-existent images or when Docker is not available,
			// the function should return 0 values without panicking
			assert.GreaterOrEqual(t, compressedSize, int64(0))
			assert.GreaterOrEqual(t, uncompressedSize, int64(0))

			// Currently the function always returns 0 for compressed size
			assert.Equal(t, int64(0), compressedSize)
		})
	}
}

// Integration test combining multiple functions
func TestImageSizeIntegration(t *testing.T) {
	tmpDir := createTestTempDir(t)
	defer cleanupTestTempDir(t, tmpDir)

	t.Run("file size measurement with lane limits", func(t *testing.T) {
		// Create a test file that's larger than Lane A limit but smaller than Lane B
		content := make([]byte, 75*1024*1024) // 75MB
		testFile := filepath.Join(tmpDir, "large_artifact.bin")
		err := os.WriteFile(testFile, content, 0644)
		require.NoError(t, err)

		// Measure size
		sizeInfo, err := GetImageSize(testFile, "", "A")
		require.NoError(t, err)

		// Get lane limit
		laneLimit, err := GetLaneSizeLimit("A")
		require.NoError(t, err)

		// Verify the file exceeds Lane A limit
		assert.Greater(t, sizeInfo.SizeMB, float64(laneLimit.MaxSizeMB),
			"File should exceed Lane A limit")

		// Check if it fits in Lane E
		laneLimitE, err := GetLaneSizeLimit("E")
		require.NoError(t, err)
		assert.Less(t, sizeInfo.SizeMB, float64(laneLimitE.MaxSizeMB),
			"File should fit in Lane E")

		// Format the size
		formattedSize := FormatSize(sizeInfo.SizeBytes)
		assert.Contains(t, formattedSize, "MB")
	})
}

// Benchmark tests
func BenchmarkParseDockerSize(b *testing.B) {
	testSizes := []string{"100MB", "2.5GB", "1024KB", "500000B"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sizeStr := testSizes[i%len(testSizes)]
		_, _ = parseDockerSize(sizeStr)
	}
}

func BenchmarkFormatSize(b *testing.B) {
	testSizes := []int64{
		1024,                      // 1KB
		1024 * 1024,               // 1MB
		1024 * 1024 * 1024,        // 1GB
		1024 * 1024 * 1024 * 1024, // 1TB
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		size := testSizes[i%len(testSizes)]
		FormatSize(size)
	}
}

func BenchmarkGetLaneSizeLimit(b *testing.B) {
	lanes := []string{"A", "B", "C", "D", "E", "F"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lane := lanes[i%len(lanes)]
		_, _ = GetLaneSizeLimit(lane)
	}
}
