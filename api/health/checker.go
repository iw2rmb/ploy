package health

import (
	"time"

	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/utils"
)

// HealthChecker provides health and readiness checking functionality
type HealthChecker struct {
	storageConfigPath string
	consulAddr        string
	nomadAddr         string
	vaultAddr         string
	metricsCollector  *HealthMetrics
	configService     *cfgsvc.Service
	disableChecks     bool
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(storageConfigPath, consulAddr, nomadAddr string) *HealthChecker {
	if nomadAddr == "" {
		nomadAddr = utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	}
	vaultAddr := utils.Getenv("VAULT_ADDR", "http://127.0.0.1:8200")

	return &HealthChecker{
		storageConfigPath: storageConfigPath,
		consulAddr:        consulAddr,
		nomadAddr:         nomadAddr,
		vaultAddr:         vaultAddr,
		metricsCollector: &HealthMetrics{
			DependencyFailures:  make(map[string]int64),
			AverageResponseTime: make(map[string]time.Duration),
		},
	}
}

// SetConfigService optionally injects centralized config service.
func (h *HealthChecker) SetConfigService(svc *cfgsvc.Service) { h.configService = svc }

// SetDependencyChecksEnabled toggles dependency health checks (used in tests).
func (h *HealthChecker) SetDependencyChecksEnabled(enabled bool) {
	h.disableChecks = !enabled
}
