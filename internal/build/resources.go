package build

import "strings"

// Lane-specific resource allocation functions

// getInstanceCountForLane returns the instance count based on lane type
func getInstanceCountForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "D":
		return 2
	default:
		return 2
	}
}

// getCpuLimitForLane returns the CPU limit based on lane type
func getCpuLimitForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "D":
		return 600
	default:
		return 600
	}
}

// getMemoryLimitForLane returns the memory limit based on lane type
func getMemoryLimitForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "D":
		return 512
	default:
		return 512
	}
}

// getJvmMemoryForLane returns the JVM memory allocation based on lane type
func getJvmMemoryForLane(lane string) int {
	return 0
}
