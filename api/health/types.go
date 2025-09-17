package health

import "time"

// HealthStatus represents the overall health status of the service
type HealthStatus struct {
	Status       string                      `json:"status"`
	Timestamp    time.Time                   `json:"timestamp"`
	Version      string                      `json:"version,omitempty"`
	Dependencies map[string]DependencyHealth `json:"dependencies"`
}

// DependencyHealth represents the health status of a dependency
type DependencyHealth struct {
	Status  string        `json:"status"`
	Latency time.Duration `json:"latency_ms"`
	Error   string        `json:"error,omitempty"`
	Details interface{}   `json:"details,omitempty"`
}

// ReadinessStatus represents the readiness status with more detailed checks
type ReadinessStatus struct {
	Ready                bool                        `json:"ready"`
	Timestamp            time.Time                   `json:"timestamp"`
	Dependencies         map[string]DependencyHealth `json:"dependencies"`
	CriticalDependencies []string                    `json:"critical_dependencies"`
}

// HealthMetrics tracks health check metrics for operational monitoring
type HealthMetrics struct {
	TotalHealthChecks    int64                    `json:"total_health_checks"`
	TotalReadinessChecks int64                    `json:"total_readiness_checks"`
	HealthyResponses     int64                    `json:"healthy_responses"`
	UnhealthyResponses   int64                    `json:"unhealthy_responses"`
	ReadyResponses       int64                    `json:"ready_responses"`
	NotReadyResponses    int64                    `json:"not_ready_responses"`
	DependencyFailures   map[string]int64         `json:"dependency_failures"`
	LastHealthCheck      time.Time                `json:"last_health_check"`
	LastReadinessCheck   time.Time                `json:"last_readiness_check"`
	AverageResponseTime  map[string]time.Duration `json:"average_response_time_ms"`
}

// DeploymentStatus represents blue-green deployment status and service mesh connectivity
type DeploymentStatus struct {
	Status              string                 `json:"status"`
	Timestamp           time.Time              `json:"timestamp"`
	DeploymentColor     string                 `json:"deployment_color"`
	DeploymentWeight    int                    `json:"deployment_weight"`
	DeploymentID        string                 `json:"deployment_id"`
	ServiceMeshEnabled  bool                   `json:"service_mesh_enabled"`
	ServiceMeshConnect  bool                   `json:"service_mesh_connect"`
	TraefikEnabled      bool                   `json:"traefik_enabled"`
	ServiceRegistration map[string]interface{} `json:"service_registration"`
}
