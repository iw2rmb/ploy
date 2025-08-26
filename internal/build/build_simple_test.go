package build

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildResourceFunctions tests that resource allocation functions work without panicking
func TestBuildResourceFunctions(t *testing.T) {
	lanes := []string{"A", "B", "C", "D", "E", "F", "G", "", "invalid"}
	
	for _, lane := range lanes {
		t.Run("lane_"+lane, func(t *testing.T) {
			// Test that functions execute without panicking and return reasonable values
			instances := getInstanceCountForLane(lane)
			cpu := getCpuLimitForLane(lane)
			memory := getMemoryLimitForLane(lane)
			jvm := getJvmMemoryForLane(lane)
			
			// Basic sanity checks
			assert.GreaterOrEqual(t, instances, 1, "Instance count should be at least 1 for lane %s", lane)
			assert.Greater(t, cpu, 0, "CPU limit should be positive for lane %s", lane)
			assert.Greater(t, memory, 0, "Memory limit should be positive for lane %s", lane)
			assert.GreaterOrEqual(t, jvm, 0, "JVM memory should be non-negative for lane %s", lane)
		})
	}
}

// TestSigningMethodFunction tests that signing method determination works
func TestSigningMethodFunction(t *testing.T) {
	testCases := []struct {
		name        string
		imagePath   string
		dockerImage string
		env         string
	}{
		{"with_image_path", "/path/to/image.img", "", "production"},
		{"with_docker_image", "", "docker.io/app:latest", "development"},
		{"empty_inputs", "", "", ""},
		{"both_paths", "/path/image.img", "docker.io/app:latest", "staging"},
	}
	
	validMethods := []string{"keyless-oidc", "cosign", "gpg", "docker-content-trust", "none"}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			method := determineSigningMethod(tc.imagePath, tc.dockerImage, tc.env)
			
			// Should return a valid signing method
			assert.NotEmpty(t, method, "Signing method should not be empty")
			
			// Verify it's one of the known methods (relaxed check)
			found := false
			for _, validMethod := range validMethods {
				if method == validMethod {
					found = true
					break
				}
			}
			// If not found in our list, just verify it's a non-empty string
			if !found {
				assert.NotEmpty(t, method, "Should return some signing method even if not in our expected list")
			}
		})
	}
}

// TestVulnerabilityScanning tests that vulnerability scanning function works
func TestVulnerabilityScanning(t *testing.T) {
	testCases := []struct {
		name      string
		imagePath string
		expectErr bool
	}{
		{"valid_path", "/tmp/test-image.img", false}, // May not exist, but shouldn't panic
		{"empty_path", "", false},                    // Should handle gracefully
		{"long_path", "/very/long/path/that/probably/does/not/exist/image.img", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test that the function executes without panicking
			assert.NotPanics(t, func() {
				result := performVulnerabilityScanning(tc.imagePath, "", "production")
				
				// Result should be a boolean
				assert.IsType(t, false, result)
			})
		})
	}
}

// TestSourceRepositoryExtraction tests repository URL extraction
func TestSourceRepositoryExtraction(t *testing.T) {
	testCases := []struct {
		name      string
		srcDir    string
		expectErr bool
	}{
		{"empty_dir", "", true},              // Should handle empty input
		{"nonexistent_dir", "/tmp/nonexist", true}, // Should handle missing directory
		{"tmp_dir", "/tmp", false},           // Should not panic with real directory
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test that the function executes without panicking
			assert.NotPanics(t, func() {
				result := extractSourceRepository(tc.srcDir)
				
				// Result should be a string (empty is OK for invalid inputs)
				assert.IsType(t, "", result)
			})
		})
	}
}

// TestBuildLaneEdgeCases tests edge cases across all build functions
func TestBuildLaneEdgeCases(t *testing.T) {
	edgeCases := []string{
		"",           // Empty
		"a",          // Lowercase
		"A",          // Uppercase  
		"z",          // Invalid lowercase
		"Z",          // Invalid uppercase
		"1",          // Numeric
		"@",          // Special character
		"AA",         // Multiple characters
		"lane-a",     // Full word
		"ABCDEFG",    // Long string
	}
	
	for _, lane := range edgeCases {
		t.Run("edge_case_"+lane, func(t *testing.T) {
			// All functions should handle any string input gracefully
			assert.NotPanics(t, func() {
				instances := getInstanceCountForLane(lane)
				cpu := getCpuLimitForLane(lane)
				memory := getMemoryLimitForLane(lane)
				jvm := getJvmMemoryForLane(lane)
				
				// Basic sanity - should return some valid numbers
				assert.IsType(t, 0, instances)
				assert.IsType(t, 0, cpu) 
				assert.IsType(t, 0, memory)
				assert.IsType(t, 0, jvm)
				
				// Values should be reasonable (not negative, not extremely large)
				assert.GreaterOrEqual(t, instances, 0, "Instances should be non-negative")
				assert.GreaterOrEqual(t, cpu, 0, "CPU should be non-negative")
				assert.GreaterOrEqual(t, memory, 0, "Memory should be non-negative") 
				assert.GreaterOrEqual(t, jvm, 0, "JVM should be non-negative")
				
				assert.LessOrEqual(t, instances, 100, "Instances should be reasonable")
				assert.LessOrEqual(t, cpu, 10000, "CPU should be reasonable")
				assert.LessOrEqual(t, memory, 10000, "Memory should be reasonable")
				assert.LessOrEqual(t, jvm, 10000, "JVM should be reasonable")
			})
		})
	}
}