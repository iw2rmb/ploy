package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ResourceConstraints represents resource limits for an application
type ResourceConstraints struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Disk   string `json:"disk,omitempty"`
}

const (
	// MinCPUMillicores is the minimum CPU in millicores (1m)
	MinCPUMillicores = 1
	
	// MaxCPUCores is the maximum CPU in cores
	MaxCPUCores = 256
	
	// MinMemoryBytes is the minimum memory in bytes (4MB)
	MinMemoryBytes = 4 * 1024 * 1024
	
	// MaxMemoryBytes is the maximum memory in bytes (1TB)
	MaxMemoryBytes = 1024 * 1024 * 1024 * 1024
	
	// MinDiskBytes is the minimum disk space in bytes (10MB)
	MinDiskBytes = 10 * 1024 * 1024
	
	// MaxDiskBytes is the maximum disk space in bytes (10TB)
	MaxDiskBytes = 10 * 1024 * 1024 * 1024 * 1024
)

// CPU format: either a decimal number (cores) or number with 'm' suffix (millicores)
var cpuRegex = regexp.MustCompile(`^(\d+(\.\d+)?|\d+m)$`)

// Memory format: number with optional unit (K, Ki, M, Mi, G, Gi, T, Ti)
var memoryRegex = regexp.MustCompile(`^(\d+)([KMGTkmgt]i?)?$`)

// Disk format: number with required unit (cannot be just bytes)
var diskRegex = regexp.MustCompile(`^(\d+)([KMGTkmgt]i?)$`)

// ValidateCPULimit validates a CPU limit string
func ValidateCPULimit(cpu string) error {
	if cpu == "" {
		return fmt.Errorf("CPU limit cannot be empty")
	}
	
	// Check format
	if !cpuRegex.MatchString(cpu) {
		return fmt.Errorf("invalid CPU limit format: %s (use number for cores or number with 'm' for millicores)", cpu)
	}
	
	// Parse value
	value, err := ParseCPUValue(cpu)
	if err != nil {
		return fmt.Errorf("invalid CPU limit: %w", err)
	}
	
	// Check for negative or zero
	if value <= 0 {
		if value < 0 {
			return fmt.Errorf("CPU limit cannot be negative")
		}
		return fmt.Errorf("CPU limit must be greater than zero")
	}
	
	// Convert to millicores for range check
	millicores := int(value * 1000)
	
	// Check minimum
	if millicores < MinCPUMillicores {
		return fmt.Errorf("CPU limit %s is below minimum (%dm)", cpu, MinCPUMillicores)
	}
	
	// Check maximum
	if value > MaxCPUCores {
		return fmt.Errorf("CPU limit %s exceeds maximum (%d cores)", cpu, MaxCPUCores)
	}
	
	return nil
}

// ValidateMemoryLimit validates a memory limit string
func ValidateMemoryLimit(memory string) error {
	if memory == "" {
		return fmt.Errorf("memory limit cannot be empty")
	}
	
	// Check for negative
	if strings.HasPrefix(memory, "-") {
		return fmt.Errorf("memory limit cannot be negative")
	}
	
	// Special handling for raw bytes (no suffix)
	if regexp.MustCompile(`^\d+$`).MatchString(memory) {
		// For raw bytes, don't allow decimal
		value, err := strconv.ParseInt(memory, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid memory limit format: %s", memory)
		}
		if value <= 0 {
			return fmt.Errorf("memory limit must be greater than zero")
		}
		if value < MinMemoryBytes {
			return fmt.Errorf("memory limit %s bytes is below minimum (%d bytes)", memory, MinMemoryBytes)
		}
		if value > MaxMemoryBytes {
			return fmt.Errorf("memory limit %s bytes exceeds maximum (%d bytes)", memory, MaxMemoryBytes)
		}
		return nil
	}
	
	// Check format with units
	if !memoryRegex.MatchString(memory) {
		return fmt.Errorf("invalid memory limit format: %s (use number with optional unit K, Ki, M, Mi, G, Gi, T, Ti)", memory)
	}
	
	// Check for decimals (not allowed with units)
	if strings.Contains(memory, ".") {
		return fmt.Errorf("decimal not allowed in memory limit with units: %s", memory)
	}
	
	// Parse value
	bytes, err := ParseMemoryValue(memory)
	if err != nil {
		return fmt.Errorf("invalid memory limit: %w", err)
	}
	
	// Check for zero
	if bytes <= 0 {
		return fmt.Errorf("memory limit must be greater than zero")
	}
	
	// Check minimum
	if bytes < MinMemoryBytes {
		return fmt.Errorf("memory limit %s is below minimum (4M)", memory)
	}
	
	// Check maximum
	if bytes > MaxMemoryBytes {
		return fmt.Errorf("memory limit %s exceeds maximum (1T)", memory)
	}
	
	return nil
}

// ValidateDiskLimit validates a disk space limit string
func ValidateDiskLimit(disk string) error {
	if disk == "" {
		return fmt.Errorf("disk limit cannot be empty")
	}
	
	// Check for negative
	if strings.HasPrefix(disk, "-") {
		return fmt.Errorf("disk limit cannot be negative")
	}
	
	// Disk must have a unit (no raw bytes)
	if regexp.MustCompile(`^\d+$`).MatchString(disk) {
		return fmt.Errorf("disk limit must specify unit (K, Ki, M, Mi, G, Gi, T, Ti)")
	}
	
	// Check format
	if !diskRegex.MatchString(disk) {
		return fmt.Errorf("invalid disk limit format: %s (use number with unit K, Ki, M, Mi, G, Gi, T, Ti)", disk)
	}
	
	// Parse value
	bytes, err := ParseMemoryValue(disk) // Same parsing as memory
	if err != nil {
		return fmt.Errorf("invalid disk limit: %w", err)
	}
	
	// Check for zero
	if bytes <= 0 {
		return fmt.Errorf("disk limit must be greater than zero")
	}
	
	// Check minimum
	if bytes < MinDiskBytes {
		return fmt.Errorf("disk limit %s is below minimum (10M)", disk)
	}
	
	// Check maximum
	if bytes > MaxDiskBytes {
		return fmt.Errorf("disk limit %s exceeds maximum (10T)", disk)
	}
	
	return nil
}

// ValidateResourceConstraints validates all resource constraints
func ValidateResourceConstraints(constraints ResourceConstraints) error {
	// Empty constraints are valid
	if constraints.CPU == "" && constraints.Memory == "" && constraints.Disk == "" {
		return nil
	}
	
	// Validate CPU if specified
	if constraints.CPU != "" {
		if err := ValidateCPULimit(constraints.CPU); err != nil {
			return fmt.Errorf("invalid CPU constraint: %w", err)
		}
	}
	
	// Validate memory if specified
	if constraints.Memory != "" {
		if err := ValidateMemoryLimit(constraints.Memory); err != nil {
			return fmt.Errorf("invalid memory constraint: %w", err)
		}
	}
	
	// Validate disk if specified
	if constraints.Disk != "" {
		if err := ValidateDiskLimit(constraints.Disk); err != nil {
			return fmt.Errorf("invalid disk constraint: %w", err)
		}
	}
	
	return nil
}

// ParseCPUValue parses a CPU value string and returns cores as float64
func ParseCPUValue(cpu string) (float64, error) {
	if cpu == "" {
		return 0, fmt.Errorf("empty CPU value")
	}
	
	// Check if it's millicores
	if strings.HasSuffix(cpu, "m") {
		millicoresStr := strings.TrimSuffix(cpu, "m")
		millicores, err := strconv.ParseFloat(millicoresStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid millicores value: %s", millicoresStr)
		}
		return millicores / 1000, nil
	}
	
	// Otherwise it's cores
	cores, err := strconv.ParseFloat(cpu, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cores value: %s", cpu)
	}
	
	return cores, nil
}

// ParseMemoryValue parses a memory value string and returns bytes
func ParseMemoryValue(memory string) (int64, error) {
	if memory == "" {
		return 0, fmt.Errorf("empty memory value")
	}
	
	// Try to match with units
	matches := memoryRegex.FindStringSubmatch(memory)
	if matches == nil {
		// Try raw bytes
		bytes, err := strconv.ParseInt(memory, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid memory format: %s", memory)
		}
		return bytes, nil
	}
	
	// Parse number part
	value, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value: %s", matches[1])
	}
	
	// No unit means bytes
	if len(matches) < 3 || matches[2] == "" {
		return value, nil
	}
	
	// Parse unit and convert to bytes
	unit := strings.ToLower(matches[2])
	var multiplier int64
	
	switch unit {
	case "k":
		multiplier = 1024
	case "ki":
		multiplier = 1024
	case "m":
		multiplier = 1024 * 1024
	case "mi":
		multiplier = 1024 * 1024
	case "g":
		multiplier = 1024 * 1024 * 1024
	case "gi":
		multiplier = 1024 * 1024 * 1024
	case "t":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "ti":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown memory unit: %s", matches[2])
	}
	
	return value * multiplier, nil
}