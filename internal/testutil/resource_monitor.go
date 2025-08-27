package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ResourceSample represents a single resource usage measurement
type ResourceSample struct {
	Timestamp  time.Time `json:"timestamp"`
	MemoryMB   int       `json:"memory_mb"`
	LoadAvg    float64   `json:"load_avg"`
	CPUPercent float64   `json:"cpu_percent,omitempty"`
}

// ResourceMonitorResult contains complete monitoring results
type ResourceMonitorResult struct {
	TestType                   string           `json:"test_type"`
	MonitoringDurationSeconds  int             `json:"monitoring_duration_seconds"`
	SampleIntervalSeconds      int             `json:"sample_interval_seconds"`
	TargetMemoryMB            int             `json:"target_memory_mb"`
	Samples                   []ResourceSample `json:"samples"`
	Summary                   ResourceSummary  `json:"summary"`
}

// ResourceSummary contains aggregated metrics
type ResourceSummary struct {
	MaxMemoryMB      int     `json:"max_memory_mb"`
	AvgMemoryMB      float64 `json:"avg_memory_mb"`
	MaxLoad          float64 `json:"max_load"`
	AvgLoad          float64 `json:"avg_load"`
	MeetsMemoryTarget bool    `json:"meets_memory_target"`
}

// ResourceMonitor provides cross-platform resource monitoring capabilities
type ResourceMonitor struct {
	testType         string
	targetMemoryMB   int
	sampleInterval   time.Duration
	processPattern   string
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor(testType string, targetMemoryMB int, sampleIntervalSec int) *ResourceMonitor {
	return &ResourceMonitor{
		testType:        testType,
		targetMemoryMB:  targetMemoryMB,
		sampleInterval:  time.Duration(sampleIntervalSec) * time.Second,
		processPattern:  "pylint-chttp", // Default pattern for CHTTP services
	}
}

// SetProcessPattern sets the process pattern to monitor
func (rm *ResourceMonitor) SetProcessPattern(pattern string) {
	rm.processPattern = pattern
}

// MonitorForDuration monitors resources for the specified duration
func (rm *ResourceMonitor) MonitorForDuration(duration time.Duration) (*ResourceMonitorResult, error) {
	samples := []ResourceSample{}
	startTime := time.Now()
	endTime := startTime.Add(duration)

	for time.Now().Before(endTime) {
		sample, err := rm.takeSample()
		if err != nil {
			// Log error but continue monitoring
			fmt.Fprintf(os.Stderr, "Warning: Failed to take resource sample: %v\n", err)
		} else {
			samples = append(samples, sample)
		}

		time.Sleep(rm.sampleInterval)
	}

	summary := rm.calculateSummary(samples)

	return &ResourceMonitorResult{
		TestType:                  rm.testType,
		MonitoringDurationSeconds: int(duration.Seconds()),
		SampleIntervalSeconds:     int(rm.sampleInterval.Seconds()),
		TargetMemoryMB:           rm.targetMemoryMB,
		Samples:                  samples,
		Summary:                  summary,
	}, nil
}

// takeSample captures a single resource usage measurement
func (rm *ResourceMonitor) takeSample() (ResourceSample, error) {
	sample := ResourceSample{
		Timestamp: time.Now(),
	}

	// Get memory usage for target processes
	memoryMB, err := rm.getProcessMemoryUsage()
	if err != nil {
		return sample, fmt.Errorf("failed to get process memory: %w", err)
	}
	sample.MemoryMB = memoryMB

	// Get system load average
	loadAvg, err := rm.getLoadAverage()
	if err != nil {
		return sample, fmt.Errorf("failed to get load average: %w", err)
	}
	sample.LoadAvg = loadAvg

	return sample, nil
}

// getProcessMemoryUsage returns memory usage in MB for processes matching the pattern
func (rm *ResourceMonitor) getProcessMemoryUsage() (int, error) {
	if rm.testType != "chttp" {
		// For non-CHTTP tests, return 0 as there are no external services to monitor
		return 0, nil
	}

	switch runtime.GOOS {
	case "linux":
		return rm.getLinuxProcessMemory()
	case "darwin":
		return rm.getDarwinProcessMemory()
	default:
		// Fallback for unsupported platforms
		return 0, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// getLinuxProcessMemory gets memory usage on Linux using /proc filesystem
func (rm *ResourceMonitor) getLinuxProcessMemory() (int, error) {
	cmd := exec.Command("pgrep", "-f", rm.processPattern)
	output, err := cmd.Output()
	if err != nil {
		// Process not found is not an error for monitoring
		return 0, nil
	}

	totalMemoryKB := 0
	pids := strings.Fields(string(output))

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
		if err != nil {
			continue
		}

		statusFile := fmt.Sprintf("/proc/%d/status", pid)
		content, err := os.ReadFile(statusFile)
		if err != nil {
			continue // Process might have terminated
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if memKB, err := strconv.Atoi(fields[1]); err == nil {
						totalMemoryKB += memKB
					}
				}
				break
			}
		}
	}

	return totalMemoryKB / 1024, nil // Convert to MB
}

// getDarwinProcessMemory gets memory usage on macOS using ps command
func (rm *ResourceMonitor) getDarwinProcessMemory() (int, error) {
	cmd := exec.Command("pgrep", "-f", rm.processPattern)
	output, err := cmd.Output()
	if err != nil {
		// Process not found is not an error for monitoring
		return 0, nil
	}

	totalMemoryKB := 0
	pids := strings.Fields(string(output))

	for _, pidStr := range pids {
		pid := strings.TrimSpace(pidStr)
		if pid == "" {
			continue
		}

		// Use ps to get RSS memory in KB
		psCmd := exec.Command("ps", "-o", "rss=", "-p", pid)
		psOutput, err := psCmd.Output()
		if err != nil {
			continue // Process might have terminated
		}

		rssStr := strings.TrimSpace(string(psOutput))
		if rss, err := strconv.Atoi(rssStr); err == nil {
			totalMemoryKB += rss
		}
	}

	return totalMemoryKB / 1024, nil // Convert to MB
}

// getLoadAverage returns the 1-minute load average
func (rm *ResourceMonitor) getLoadAverage() (float64, error) {
	switch runtime.GOOS {
	case "linux", "darwin":
		return rm.getUnixLoadAverage()
	default:
		return 0.0, fmt.Errorf("load average not supported on %s", runtime.GOOS)
	}
}

// getUnixLoadAverage gets load average on Unix-like systems
func (rm *ResourceMonitor) getUnixLoadAverage() (float64, error) {
	cmd := exec.Command("uptime")
	output, err := cmd.Output()
	if err != nil {
		return 0.0, err
	}

	uptimeStr := string(output)
	
	// Parse load average from uptime output
	// Example: "load average: 1.23, 0.56, 0.78" or "load averages: 1.23 0.56 0.78"
	if strings.Contains(uptimeStr, "load average:") {
		parts := strings.Split(uptimeStr, "load average:")
		if len(parts) >= 2 {
			loadPart := strings.TrimSpace(parts[1])
			loadValues := strings.Split(loadPart, ",")
			if len(loadValues) >= 1 {
				loadStr := strings.TrimSpace(loadValues[0])
				return strconv.ParseFloat(loadStr, 64)
			}
		}
	} else if strings.Contains(uptimeStr, "load averages:") {
		parts := strings.Split(uptimeStr, "load averages:")
		if len(parts) >= 2 {
			loadPart := strings.TrimSpace(parts[1])
			loadValues := strings.Fields(loadPart)
			if len(loadValues) >= 1 {
				return strconv.ParseFloat(loadValues[0], 64)
			}
		}
	}

	return 0.0, fmt.Errorf("could not parse load average from uptime output")
}

// calculateSummary computes summary statistics from samples
func (rm *ResourceMonitor) calculateSummary(samples []ResourceSample) ResourceSummary {
	if len(samples) == 0 {
		return ResourceSummary{}
	}

	var maxMemory int
	var totalMemory int
	var maxLoad float64
	var totalLoad float64

	for _, sample := range samples {
		if sample.MemoryMB > maxMemory {
			maxMemory = sample.MemoryMB
		}
		totalMemory += sample.MemoryMB

		if sample.LoadAvg > maxLoad {
			maxLoad = sample.LoadAvg
		}
		totalLoad += sample.LoadAvg
	}

	avgMemory := float64(totalMemory) / float64(len(samples))
	avgLoad := totalLoad / float64(len(samples))

	return ResourceSummary{
		MaxMemoryMB:       maxMemory,
		AvgMemoryMB:       avgMemory,
		MaxLoad:           maxLoad,
		AvgLoad:           avgLoad,
		MeetsMemoryTarget: maxMemory <= rm.targetMemoryMB,
	}
}

// SaveToFile saves the monitoring result to a JSON file
func (result *ResourceMonitorResult) SaveToFile(filepath string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	return os.WriteFile(filepath, data, 0644)
}