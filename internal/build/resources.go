package build

import "strings"

// Lane-specific resource allocation functions

// getInstanceCountForLane returns the instance count based on lane type
func getInstanceCountForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "A", "B": // Unikraft - can run more instances due to low memory footprint
		return 3
	case "C": // OSv/JVM - fewer instances due to higher memory usage
		return 2
	case "D": // FreeBSD jail - moderate resource usage
		return 2
	case "E": // OCI with Kontain - good isolation, moderate overhead
		return 2
	case "F": // Full VM - resource intensive
		return 1
	default:
		return 2
	}
}

// getCpuLimitForLane returns the CPU limit based on lane type
func getCpuLimitForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "A", "B": // Unikraft - very efficient, needs minimal CPU
		return 200
	case "C": // OSv/JVM - needs more CPU for JIT compilation and GC
		return 1000
	case "D": // FreeBSD jail - native performance
		return 500
	case "E": // OCI with Kontain - good performance with slight overhead
		return 600
	case "F": // Full VM - higher overhead
		return 800
	default:
		return 500
	}
}

// getMemoryLimitForLane returns the memory limit based on lane type
func getMemoryLimitForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "A", "B": // Unikraft - extremely memory efficient
		return 128
	case "C": // OSv/JVM - needs memory for heap, metaspace, and JIT
		return 1024
	case "D": // FreeBSD jail - moderate memory usage
		return 256
	case "E": // OCI with Kontain - container plus isolation overhead
		return 512
	case "F": // Full VM - highest memory overhead
		return 2048
	default:
		return 256
	}
}

// getJvmMemoryForLane returns the JVM memory allocation based on lane type
func getJvmMemoryForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "C": // OSv/JVM - dedicated JVM memory allocation
		return 768 // Leave room for OS and JVM overhead
	default:
		return 0 // No JVM memory for non-JVM lanes
	}
}
