package build

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLaneResourceAllocation(t *testing.T) {
	tests := []struct {
		name                  string
		lane                  string
		expectedInstanceCount int
		expectedCpuLimit      int
		expectedMemoryLimit   int
		expectedJvmMemory     int
	}{
		{
			name:                  "Lane A - Unikraft",
			lane:                  "A",
			expectedInstanceCount: 3,
			expectedCpuLimit:      200,
			expectedMemoryLimit:   128,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane B - Unikraft",
			lane:                  "B",
			expectedInstanceCount: 3,
			expectedCpuLimit:      200,
			expectedMemoryLimit:   128,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane C - OSv/JVM",
			lane:                  "C",
			expectedInstanceCount: 2,
			expectedCpuLimit:      1000,
			expectedMemoryLimit:   1024,
			expectedJvmMemory:     768,
		},
		{
			name:                  "Lane D - FreeBSD jail",
			lane:                  "D",
			expectedInstanceCount: 2,
			expectedCpuLimit:      500,
			expectedMemoryLimit:   256,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane E - OCI with Kontain",
			lane:                  "E",
			expectedInstanceCount: 2,
			expectedCpuLimit:      600,
			expectedMemoryLimit:   512,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane F - Full VM",
			lane:                  "F",
			expectedInstanceCount: 1,
			expectedCpuLimit:      800,
			expectedMemoryLimit:   2048,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Default lane (lowercase)",
			lane:                  "unknown",
			expectedInstanceCount: 2,
			expectedCpuLimit:      500,
			expectedMemoryLimit:   256,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Case insensitive - lowercase c",
			lane:                  "c",
			expectedInstanceCount: 2,
			expectedCpuLimit:      1000,
			expectedMemoryLimit:   1024,
			expectedJvmMemory:     768,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instanceCount := getInstanceCountForLane(tt.lane)
			assert.Equal(t, tt.expectedInstanceCount, instanceCount)

			cpuLimit := getCpuLimitForLane(tt.lane)
			assert.Equal(t, tt.expectedCpuLimit, cpuLimit)

			memoryLimit := getMemoryLimitForLane(tt.lane)
			assert.Equal(t, tt.expectedMemoryLimit, memoryLimit)

			jvmMemory := getJvmMemoryForLane(tt.lane)
			assert.Equal(t, tt.expectedJvmMemory, jvmMemory)
		})
	}
}

// Test helper functions for lane resources consistency
func TestLaneResourceConsistency(t *testing.T) {
	lanes := []string{"A", "B", "C", "D", "E", "F", "G"}

	for _, lane := range lanes {
		t.Run(fmt.Sprintf("Lane_%s_consistency", lane), func(t *testing.T) {
			instanceCount := getInstanceCountForLane(lane)
			cpuLimit := getCpuLimitForLane(lane)
			memoryLimit := getMemoryLimitForLane(lane)
			jvmMemory := getJvmMemoryForLane(lane)

			// Validate ranges
			assert.True(t, instanceCount >= 1 && instanceCount <= 5,
				"Instance count should be between 1 and 5, got %d", instanceCount)

			assert.True(t, cpuLimit >= 100 && cpuLimit <= 2000,
				"CPU limit should be between 100 and 2000, got %d", cpuLimit)

			assert.True(t, memoryLimit >= 64 && memoryLimit <= 4096,
				"Memory limit should be between 64 and 4096, got %d", memoryLimit)

			assert.True(t, jvmMemory >= 0 && jvmMemory <= memoryLimit,
				"JVM memory should be between 0 and memory limit (%d), got %d", memoryLimit, jvmMemory)

			// Specific validations for JVM lane
			if strings.ToUpper(lane) == "C" {
				assert.Greater(t, jvmMemory, 0, "Lane C should have JVM memory allocation")
				assert.Less(t, jvmMemory, memoryLimit, "JVM memory should be less than total memory limit")
			} else {
				assert.Equal(t, 0, jvmMemory, "Non-JVM lanes should have zero JVM memory")
			}
		})
	}
}

// Property-based testing
func TestLaneResourceProperties(t *testing.T) {
	t.Run("memory efficiency order", func(t *testing.T) {
		// Unikraft (A,B) should be most memory efficient
		unikraftMemory := getMemoryLimitForLane("A")
		jailMemory := getMemoryLimitForLane("D")
		containerMemory := getMemoryLimitForLane("E")
		jvmMemory := getMemoryLimitForLane("C")
		vmMemory := getMemoryLimitForLane("F")

		// Verify expected memory efficiency order
		assert.Less(t, unikraftMemory, jailMemory, "Unikraft should be more memory efficient than jails")
		assert.Less(t, jailMemory, containerMemory, "Jails should be more memory efficient than containers")
		assert.Less(t, containerMemory, jvmMemory, "Containers should be more memory efficient than JVM")
		assert.Less(t, jvmMemory, vmMemory, "JVM should be more memory efficient than full VMs")
	})

	t.Run("CPU efficiency considerations", func(t *testing.T) {
		// Unikraft should need least CPU due to efficiency
		unikraftCPU := getCpuLimitForLane("A")
		jailCPU := getCpuLimitForLane("D")
		containerCPU := getCpuLimitForLane("E")
		jvmCPU := getCpuLimitForLane("C")

		// Unikraft should be most CPU efficient
		assert.Less(t, unikraftCPU, jailCPU, "Unikraft should need less CPU than jails")
		assert.Less(t, jailCPU, containerCPU, "Native jails should need less CPU than containers")

		// JVM needs most CPU for JIT compilation and GC
		assert.Greater(t, jvmCPU, containerCPU, "JVM should need more CPU than containers")
	})

	t.Run("instance scaling relationship", func(t *testing.T) {
		// More efficient lanes should support more instances
		unikraftInstances := getInstanceCountForLane("A")
		vmInstances := getInstanceCountForLane("F")

		assert.Greater(t, unikraftInstances, vmInstances, "More efficient lanes should support more instances")
	})
}

// Edge case testing
func TestLaneResourceEdgeCases(t *testing.T) {
	t.Run("case insensitivity", func(t *testing.T) {
		upperA := getInstanceCountForLane("A")
		lowerA := getInstanceCountForLane("a")
		assert.Equal(t, upperA, lowerA, "Lane resource allocation should be case insensitive")

		upperC := getCpuLimitForLane("C")
		lowerC := getCpuLimitForLane("c")
		assert.Equal(t, upperC, lowerC, "Lane resource allocation should be case insensitive")
	})

	t.Run("unknown lanes default consistently", func(t *testing.T) {
		unknownInstances := getInstanceCountForLane("unknown")
		emptyInstances := getInstanceCountForLane("")
		invalidInstances := getInstanceCountForLane("XYZ")

		// All unknown lanes should get the same default values
		assert.Equal(t, unknownInstances, emptyInstances, "All unknown lanes should get same defaults")
		assert.Equal(t, emptyInstances, invalidInstances, "All unknown lanes should get same defaults")
	})
}

// Benchmarks for resource allocation functions
func BenchmarkGetInstanceCountForLane(b *testing.B) {
	lanes := []string{"A", "B", "C", "D", "E", "F"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, lane := range lanes {
			getInstanceCountForLane(lane)
		}
	}
}

func BenchmarkGetCpuLimitForLane(b *testing.B) {
	lanes := []string{"A", "B", "C", "D", "E", "F"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, lane := range lanes {
			getCpuLimitForLane(lane)
		}
	}
}

func BenchmarkGetMemoryLimitForLane(b *testing.B) {
	lanes := []string{"A", "B", "C", "D", "E", "F"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, lane := range lanes {
			getMemoryLimitForLane(lane)
		}
	}
}
