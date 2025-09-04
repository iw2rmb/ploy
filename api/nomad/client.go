package nomad

import (
    "time"
    orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// WaitHealthy delegates to the unified internal orchestration monitor.
func WaitHealthy(jobName string, timeout time.Duration) error {
    monitor := orchestration.NewHealthMonitor()
    return monitor.WaitForHealthyAllocations(jobName, 1, timeout)
}

// GetJobEndpoint delegates to the unified internal orchestration monitor.
func GetJobEndpoint(jobName string) (string, error) {
    monitor := orchestration.NewHealthMonitor()
    return monitor.GetJobEndpoint(jobName)
}

// IsJobHealthy delegates to the unified internal orchestration monitor.
func IsJobHealthy(jobName string) bool {
    monitor := orchestration.NewHealthMonitor()
    return monitor.IsJobHealthy(jobName)
}
