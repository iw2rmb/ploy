package metrics

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all the Prometheus metrics for the controller
type Metrics struct {
	// Request metrics
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec

	// Leadership metrics
	IsLeader          prometheus.Gauge
	LeadershipChanges *prometheus.CounterVec

	// Application metrics
	ActiveApps    prometheus.Gauge
	BuildsTotal   *prometheus.CounterVec
	BuildDuration *prometheus.HistogramVec

	// System metrics
	ControllerUptime prometheus.Gauge

	// Coordination metrics
	TTLCleanupRuns *prometheus.CounterVec
	TTLCleanedJobs prometheus.Counter

	// Storage metrics
	StorageOperations *prometheus.CounterVec
	StorageErrors     *prometheus.CounterVec

	// Certificate metrics
	CertificatesTotal     prometheus.Gauge
	CertificateOperations *prometheus.CounterVec
	CertificateExpiry     *prometheus.GaugeVec

	// Performance metrics
	CacheHitRate        *prometheus.GaugeVec
	CacheOperations     *prometheus.CounterVec
	ConnectionPoolUsage *prometheus.GaugeVec
	ConnectionPoolOps   *prometheus.CounterVec
	ConfigLoadTime      *prometheus.HistogramVec
	StartupTime         prometheus.Gauge

	registry  *prometheus.Registry
	startTime time.Time
	logger    *log.Logger
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	logger := log.New(os.Stdout, "[metrics] ", log.LstdFlags|log.Lshortfile)
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry:  registry,
		startTime: time.Now(),
		logger:    logger,
	}

	// Initialize metrics
	m.initializeMetrics()

	// Register metrics with registry
	m.registerMetrics()

	logger.Println("Prometheus metrics initialized")
	return m
}

// initializeMetrics initializes all Prometheus metrics
func (m *Metrics) initializeMetrics() {
	// Request metrics
	m.RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_requests_total",
			Help: "Total number of HTTP requests processed by the controller",
		},
		[]string{"method", "endpoint", "status_code"},
	)

	m.RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ploy_api_request_duration_seconds",
			Help:    "Duration of HTTP requests processed by the controller",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// Leadership metrics
	m.IsLeader = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ploy_api_is_leader",
			Help: "Whether this controller instance is the current leader (1 for leader, 0 for follower)",
		},
	)

	m.LeadershipChanges = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_leadership_changes_total",
			Help: "Total number of leadership changes",
		},
		[]string{"type"}, // gained, lost
	)

	// Application metrics
	m.ActiveApps = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ploy_api_active_apps",
			Help: "Number of active applications",
		},
	)

	m.BuildsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_builds_total",
			Help: "Total number of application builds",
		},
		[]string{"app", "lane", "status"}, // success, failure
	)

	m.BuildDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ploy_api_build_duration_seconds",
			Help:    "Duration of application builds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1200}, // 10s to 20min
		},
		[]string{"app", "lane"},
	)

	// System metrics
	m.ControllerUptime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ploy_api_uptime_seconds",
			Help: "Controller uptime in seconds",
		},
	)

	// Coordination metrics
	m.TTLCleanupRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_ttl_cleanup_runs_total",
			Help: "Total number of TTL cleanup runs",
		},
		[]string{"status"}, // success, error
	)

	m.TTLCleanedJobs = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ploy_api_ttl_cleaned_jobs_total",
			Help: "Total number of jobs cleaned up by TTL cleanup",
		},
	)

	// Storage metrics
	m.StorageOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_storage_operations_total",
			Help: "Total number of storage operations",
		},
		[]string{"operation", "status"}, // upload, download, delete; success, error
	)

	m.StorageErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_storage_errors_total",
			Help: "Total number of storage errors",
		},
		[]string{"operation", "error_type"},
	)

	// Certificate metrics
	m.CertificatesTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ploy_api_certificates_total",
			Help: "Total number of certificates managed",
		},
	)

	m.CertificateOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_certificate_operations_total",
			Help: "Total number of certificate operations",
		},
		[]string{"operation", "status"}, // provision, renew, revoke; success, error
	)

	m.CertificateExpiry = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ploy_api_certificate_expiry_seconds",
			Help: "Time until certificate expiry in seconds",
		},
		[]string{"domain", "app"},
	)

	// Performance metrics
	m.CacheHitRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ploy_api_cache_hit_rate",
			Help: "Cache hit rate percentage",
		},
		[]string{"cache_type"}, // envstore, config, etc.
	)

	m.CacheOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_cache_operations_total",
			Help: "Total number of cache operations",
		},
		[]string{"cache_type", "operation"}, // get, set, delete, clear
	)

	m.ConnectionPoolUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ploy_api_connection_pool_usage",
			Help: "Current connection pool usage",
		},
		[]string{"service"}, // consul, nomad, storage
	)

	m.ConnectionPoolOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ploy_api_connection_pool_operations_total",
			Help: "Total number of connection pool operations",
		},
		[]string{"service", "operation"}, // acquire, release, create
	)

	m.ConfigLoadTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ploy_api_config_load_duration_seconds",
			Help:    "Time spent loading configuration files",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1}, // 1ms to 1s
		},
		[]string{"config_type", "cached"}, // storage, cleanup; true, false
	)

	m.StartupTime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ploy_api_startup_duration_seconds",
			Help: "Time taken for controller startup",
		},
	)
}

// registerMetrics registers all metrics with the Prometheus registry
func (m *Metrics) registerMetrics() {
	metrics := []prometheus.Collector{
		m.RequestsTotal,
		m.RequestDuration,
		m.IsLeader,
		m.LeadershipChanges,
		m.ActiveApps,
		m.BuildsTotal,
		m.BuildDuration,
		m.ControllerUptime,
		m.TTLCleanupRuns,
		m.TTLCleanedJobs,
		m.StorageOperations,
		m.StorageErrors,
		m.CertificatesTotal,
		m.CertificateOperations,
		m.CertificateExpiry,
		m.CacheHitRate,
		m.CacheOperations,
		m.ConnectionPoolUsage,
		m.ConnectionPoolOps,
		m.ConfigLoadTime,
		m.StartupTime,
	}

	for _, metric := range metrics {
		if err := m.registry.Register(metric); err != nil {
			m.logger.Printf("Failed to register metric: %v", err)
		}
	}
}

// UpdateUptime updates the controller uptime metric
func (m *Metrics) UpdateUptime() {
	uptime := time.Since(m.startTime).Seconds()
	m.ControllerUptime.Set(uptime)
}

// SetLeaderStatus sets the leadership status
func (m *Metrics) SetLeaderStatus(isLeader bool) {
	if isLeader {
		m.IsLeader.Set(1)
	} else {
		m.IsLeader.Set(0)
	}
}

// RecordLeadershipChange records a leadership change
func (m *Metrics) RecordLeadershipChange(changeType string) {
	m.LeadershipChanges.WithLabelValues(changeType).Inc()
}

// RecordRequest records an HTTP request
func (m *Metrics) RecordRequest(method, endpoint, statusCode string, duration time.Duration) {
	m.RequestsTotal.WithLabelValues(method, endpoint, statusCode).Inc()
	m.RequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

// RecordBuild records an application build
func (m *Metrics) RecordBuild(app, lane, status string, duration time.Duration) {
	m.BuildsTotal.WithLabelValues(app, lane, status).Inc()
	if status == "success" {
		m.BuildDuration.WithLabelValues(app, lane).Observe(duration.Seconds())
	}
}

// UpdateActiveApps updates the active applications count
func (m *Metrics) UpdateActiveApps(count float64) {
	m.ActiveApps.Set(count)
}

// RecordTTLCleanupRun records a TTL cleanup run
func (m *Metrics) RecordTTLCleanupRun(status string, cleanedJobs int) {
	m.TTLCleanupRuns.WithLabelValues(status).Inc()
	if status == "success" && cleanedJobs > 0 {
		m.TTLCleanedJobs.Add(float64(cleanedJobs))
	}
}

// RecordStorageOperation records a storage operation
func (m *Metrics) RecordStorageOperation(operation, status string) {
	m.StorageOperations.WithLabelValues(operation, status).Inc()
}

// RecordStorageError records a storage error
func (m *Metrics) RecordStorageError(operation, errorType string) {
	m.StorageErrors.WithLabelValues(operation, errorType).Inc()
}

// RecordCertificateOperation records a certificate operation
func (m *Metrics) RecordCertificateOperation(operation, status string) {
	m.CertificateOperations.WithLabelValues(operation, status).Inc()
}

// UpdateCertificatesTotal updates the total certificates count
func (m *Metrics) UpdateCertificatesTotal(count float64) {
	m.CertificatesTotal.Set(count)
}

// UpdateCertificateExpiry updates certificate expiry time
func (m *Metrics) UpdateCertificateExpiry(domain, app string, expiryTime time.Time) {
	secondsUntilExpiry := time.Until(expiryTime).Seconds()
	m.CertificateExpiry.WithLabelValues(domain, app).Set(secondsUntilExpiry)
}

// Handler returns a Fiber handler for the Prometheus metrics endpoint
func (m *Metrics) Handler() fiber.Handler {
	promHandler := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry: m.registry,
	})

	return adaptor.HTTPHandler(promHandler)
}

// HTTPHandler returns an HTTP handler for the Prometheus metrics endpoint
func (m *Metrics) HTTPHandler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry: m.registry,
	})
}

// MetricsMiddleware creates Fiber middleware to record request metrics
func (m *Metrics) MetricsMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Process request
		err := c.Next()

		// Record metrics
		duration := time.Since(start)
		method := c.Method()
		path := c.Route().Path
		statusCode := c.Response().StatusCode()

		// Sanitize path for metrics (remove parameters)
		endpoint := sanitizeEndpoint(path)

		m.RecordRequest(method, endpoint, string(rune(statusCode)), duration)

		return err
	}
}

// sanitizeEndpoint removes parameter values from endpoints for metrics
func sanitizeEndpoint(path string) string {
	// This is a simple implementation - could be enhanced with regex
	// Replace common parameter patterns
	// Examples: /v1/apps/myapp -> /v1/apps/:app
	//          /v1/apps/myapp/builds/123 -> /v1/apps/:app/builds/:id

	// For now, return the raw path - this can be enhanced later
	return path
}

// StartUptimeUpdater starts a background goroutine to update uptime metrics
func (m *Metrics) StartUptimeUpdater() {
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()
		for range ticker.C {
			m.UpdateUptime()
		}
	}()
}

// Performance metrics recording methods

// RecordCacheHitRate records cache hit rate percentage
func (m *Metrics) RecordCacheHitRate(cacheType string, hitRate float64) {
	m.CacheHitRate.WithLabelValues(cacheType).Set(hitRate)
}

// RecordCacheOperation records cache operations
func (m *Metrics) RecordCacheOperation(cacheType, operation string) {
	m.CacheOperations.WithLabelValues(cacheType, operation).Inc()
}

// UpdateConnectionPoolUsage updates connection pool usage
func (m *Metrics) UpdateConnectionPoolUsage(service string, usage float64) {
	m.ConnectionPoolUsage.WithLabelValues(service).Set(usage)
}

// RecordConnectionPoolOperation records connection pool operations
func (m *Metrics) RecordConnectionPoolOperation(service, operation string) {
	m.ConnectionPoolOps.WithLabelValues(service, operation).Inc()
}

// RecordConfigLoadTime records configuration loading time
func (m *Metrics) RecordConfigLoadTime(configType string, cached bool, duration time.Duration) {
	cachedStr := "false"
	if cached {
		cachedStr = "true"
	}
	m.ConfigLoadTime.WithLabelValues(configType, cachedStr).Observe(duration.Seconds())
}

// RecordStartupTime records controller startup time
func (m *Metrics) RecordStartupTime(duration time.Duration) {
	m.StartupTime.Set(duration.Seconds())
}
