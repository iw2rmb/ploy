package arf

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertType represents the type of healing alert
type AlertType string

const (
	AlertTypeFailureRate    AlertType = "failure_rate"
	AlertTypeDeepHierarchy  AlertType = "deep_hierarchy"
	AlertTypeLongRunning    AlertType = "long_running"
	AlertTypeCircuitBreaker AlertType = "circuit_breaker"
	AlertTypeConsulError    AlertType = "consul_error"
)

// HealingAlert represents an alert triggered by healing workflow issues
type HealingAlert struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Severity   string                 `json:"severity"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Resolved   bool                   `json:"resolved"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
}

// AlertConfig configures the healing alert system
type AlertConfig struct {
	Enabled              bool          `json:"enabled"`
	FailureRateThreshold float64       `json:"failure_rate_threshold"` // Default: 0.8 (80%)
	MaxTreeDepth         int           `json:"max_tree_depth"`         // Default: 8
	MaxDuration          time.Duration `json:"max_duration"`           // Default: 4h
	EvaluationInterval   time.Duration `json:"evaluation_interval"`    // Default: 1m
	DeduplicationWindow  time.Duration `json:"deduplication_window"`   // Default: 5m
	MaxHistorySize       int           `json:"max_history_size"`       // Default: 100
	WebhookURL           string        `json:"webhook_url,omitempty"`  // Optional webhook for alerts
}

// Validate checks if the alert configuration is valid
func (c *AlertConfig) Validate() error {
	if c.FailureRateThreshold < 0 || c.FailureRateThreshold > 1 {
		return fmt.Errorf("failure rate threshold must be between 0 and 1")
	}
	if c.MaxTreeDepth < 0 {
		return fmt.Errorf("max tree depth must be non-negative")
	}
	if c.MaxDuration < 0 {
		return fmt.Errorf("max duration must be non-negative")
	}
	return nil
}

// HealingAlertManager manages alert evaluation and notification
type HealingAlertManager struct {
	config         *AlertConfig
	activeAlerts   map[string]*HealingAlert
	alertHistory   []*HealingAlert
	alertCallbacks []func(*HealingAlert)
	lastAlertTime  map[string]time.Time // For deduplication
	treeDepths     map[string]int       // Track max depth per transformation
	startTimes     map[string]time.Time // Track transformation start times

	mu      sync.RWMutex
	running int32 // atomic
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Metrics for evaluation
	lastMetrics HealingCoordinatorMetrics
	metricsMu   sync.RWMutex
}

// NewHealingAlertManager creates a new alert manager
func NewHealingAlertManager(config *AlertConfig) *HealingAlertManager {
	if config == nil {
		config = &AlertConfig{
			Enabled:              true,
			FailureRateThreshold: 0.8,
			MaxTreeDepth:         8,
			MaxDuration:          4 * time.Hour,
			EvaluationInterval:   1 * time.Minute,
			DeduplicationWindow:  5 * time.Minute,
			MaxHistorySize:       100,
		}
	}

	if config.MaxHistorySize == 0 {
		config.MaxHistorySize = 100
	}
	if config.DeduplicationWindow == 0 {
		config.DeduplicationWindow = 5 * time.Minute
	}

	return &HealingAlertManager{
		config:         config,
		activeAlerts:   make(map[string]*HealingAlert),
		alertHistory:   make([]*HealingAlert, 0, config.MaxHistorySize),
		alertCallbacks: make([]func(*HealingAlert), 0),
		lastAlertTime:  make(map[string]time.Time),
		treeDepths:     make(map[string]int),
		startTimes:     make(map[string]time.Time),
	}
}

// GetConfig returns the current alert configuration
func (m *HealingAlertManager) GetConfig() *AlertConfig {
	return m.config
}

// RegisterAlertCallback registers a callback for when alerts are triggered
func (m *HealingAlertManager) RegisterAlertCallback(callback func(*HealingAlert)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertCallbacks = append(m.alertCallbacks, callback)
}

// EvaluateRules evaluates alert rules based on current metrics
func (m *HealingAlertManager) EvaluateRules(metrics HealingCoordinatorMetrics) {
	if !m.config.Enabled {
		return
	}

	// Store metrics for evaluation
	m.metricsMu.Lock()
	m.lastMetrics = metrics
	m.metricsMu.Unlock()

	// Evaluate each rule
	m.evaluateFailureRate(metrics)
	m.evaluateDeepHierarchy()
	m.evaluateLongRunning()
	m.evaluateCircuitBreaker(metrics)

	// Check for resolved alerts
	m.checkResolvedAlerts(metrics)
}

// evaluateFailureRate checks if failure rate exceeds threshold
func (m *HealingAlertManager) evaluateFailureRate(metrics HealingCoordinatorMetrics) {
	total := metrics.CompletedTasks + metrics.FailedTasks
	if total == 0 {
		return
	}

	failureRate := float64(metrics.FailedTasks) / float64(total)

	if failureRate > m.config.FailureRateThreshold {
		alert := &HealingAlert{
			ID:       "failure_rate_high",
			Type:     string(AlertTypeFailureRate),
			Severity: string(AlertSeverityCritical),
			Message: fmt.Sprintf("Healing failure rate (%.1f%%) exceeds threshold (%.1f%%)",
				failureRate*100, m.config.FailureRateThreshold*100),
			Details: map[string]interface{}{
				"failure_rate":    failureRate,
				"failed_tasks":    metrics.FailedTasks,
				"completed_tasks": metrics.CompletedTasks,
				"threshold":       m.config.FailureRateThreshold,
			},
			Timestamp: time.Now(),
			Resolved:  false,
		}

		m.triggerAlert(alert)
	}
}

// evaluateDeepHierarchy checks for excessively deep healing trees
func (m *HealingAlertManager) evaluateDeepHierarchy() {
	m.mu.RLock()
	depths := make(map[string]int)
	for id, depth := range m.treeDepths {
		depths[id] = depth
	}
	m.mu.RUnlock()

	for transformID, depth := range depths {
		if depth > m.config.MaxTreeDepth {
			alert := &HealingAlert{
				ID:       fmt.Sprintf("deep_hierarchy_%s", transformID),
				Type:     string(AlertTypeDeepHierarchy),
				Severity: string(AlertSeverityWarning),
				Message: fmt.Sprintf("Healing tree depth (%d) exceeds maximum (%d) for transformation %s",
					depth, m.config.MaxTreeDepth, transformID),
				Details: map[string]interface{}{
					"transformation_id": transformID,
					"depth":             depth,
					"max_depth":         m.config.MaxTreeDepth,
				},
				Timestamp: time.Now(),
				Resolved:  false,
			}

			m.triggerAlert(alert)
		}
	}
}

// evaluateLongRunning checks for transformations running too long
func (m *HealingAlertManager) evaluateLongRunning() {
	m.mu.RLock()
	startTimes := make(map[string]time.Time)
	for id, start := range m.startTimes {
		startTimes[id] = start
	}
	m.mu.RUnlock()

	now := time.Now()
	for transformID, startTime := range startTimes {
		duration := now.Sub(startTime)
		if duration > m.config.MaxDuration {
			alert := &HealingAlert{
				ID:       fmt.Sprintf("long_running_%s", transformID),
				Type:     string(AlertTypeLongRunning),
				Severity: string(AlertSeverityWarning),
				Message: fmt.Sprintf("Transformation %s has been running for %v (max: %v)",
					transformID, duration.Round(time.Minute), m.config.MaxDuration),
				Details: map[string]interface{}{
					"transformation_id": transformID,
					"duration":          duration.String(),
					"max_duration":      m.config.MaxDuration.String(),
					"start_time":        startTime,
				},
				Timestamp: time.Now(),
				Resolved:  false,
			}

			m.triggerAlert(alert)
		}
	}
}

// evaluateCircuitBreaker checks circuit breaker state
func (m *HealingAlertManager) evaluateCircuitBreaker(metrics HealingCoordinatorMetrics) {
	if metrics.CircuitBreakerState == "open" {
		alert := &HealingAlert{
			ID:       "circuit_breaker_open",
			Type:     string(AlertTypeCircuitBreaker),
			Severity: string(AlertSeverityWarning),
			Message: fmt.Sprintf("Circuit breaker is open after %d consecutive failures",
				metrics.ConsecutiveFailures),
			Details: map[string]interface{}{
				"state":                metrics.CircuitBreakerState,
				"consecutive_failures": metrics.ConsecutiveFailures,
				"open_until":           metrics.CircuitOpenUntil,
			},
			Timestamp: time.Now(),
			Resolved:  false,
		}

		m.triggerAlert(alert)
	}
}

// checkResolvedAlerts checks if any active alerts should be resolved
func (m *HealingAlertManager) checkResolvedAlerts(metrics HealingCoordinatorMetrics) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check failure rate alert
	total := metrics.CompletedTasks + metrics.FailedTasks
	if total > 0 {
		failureRate := float64(metrics.FailedTasks) / float64(total)
		if failureRate <= m.config.FailureRateThreshold {
			if alert, exists := m.activeAlerts["failure_rate_high"]; exists && !alert.Resolved {
				m.resolveAlert(alert)
			}
		}
	}

	// Check circuit breaker alert
	if metrics.CircuitBreakerState != "open" {
		if alert, exists := m.activeAlerts["circuit_breaker_open"]; exists && !alert.Resolved {
			m.resolveAlert(alert)
		}
	}
}

// triggerAlert triggers an alert if not already active or deduplicated
func (m *HealingAlertManager) triggerAlert(alert *HealingAlert) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for deduplication
	if lastTime, exists := m.lastAlertTime[alert.ID]; exists {
		if time.Since(lastTime) < m.config.DeduplicationWindow {
			return // Deduplicate
		}
	}

	// Check if alert already active
	if existing, exists := m.activeAlerts[alert.ID]; exists && !existing.Resolved {
		return // Already active
	}

	// Record alert
	m.activeAlerts[alert.ID] = alert
	m.lastAlertTime[alert.ID] = time.Now()
	m.addToHistory(alert)

	// Trigger callbacks asynchronously
	for _, callback := range m.alertCallbacks {
		go callback(alert)
	}
}

// resolveAlert marks an alert as resolved
func (m *HealingAlertManager) resolveAlert(alert *HealingAlert) {
	now := time.Now()
	alert.Resolved = true
	alert.ResolvedAt = &now

	// Trigger resolution callbacks
	for _, callback := range m.alertCallbacks {
		go callback(alert)
	}
}

// addToHistory adds an alert to history, maintaining max size
func (m *HealingAlertManager) addToHistory(alert *HealingAlert) {
	m.alertHistory = append(m.alertHistory, alert)

	// Trim history if needed
	if len(m.alertHistory) > m.config.MaxHistorySize {
		// Keep the most recent alerts
		start := len(m.alertHistory) - m.config.MaxHistorySize
		m.alertHistory = m.alertHistory[start:]
	}
}

// RecordTreeDepth records the depth of a healing tree
func (m *HealingAlertManager) RecordTreeDepth(transformID string, depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track maximum depth seen for this transformation
	if currentDepth, exists := m.treeDepths[transformID]; !exists || depth > currentDepth {
		m.treeDepths[transformID] = depth
	}
}

// StartTracking starts tracking a transformation's duration
func (m *HealingAlertManager) StartTracking(transformID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startTimes[transformID] = time.Now()
}

// StopTracking stops tracking a transformation and resolves any related alerts
func (m *HealingAlertManager) StopTracking(transformID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.startTimes, transformID)
	delete(m.treeDepths, transformID)

	// Resolve any long-running alert for this transformation
	alertID := fmt.Sprintf("long_running_%s", transformID)
	if alert, exists := m.activeAlerts[alertID]; exists && !alert.Resolved {
		m.resolveAlert(alert)
	}

	// Resolve any deep hierarchy alert for this transformation
	alertID = fmt.Sprintf("deep_hierarchy_%s", transformID)
	if alert, exists := m.activeAlerts[alertID]; exists && !alert.Resolved {
		m.resolveAlert(alert)
	}
}

// GetActiveAlerts returns all currently active alerts
func (m *HealingAlertManager) GetActiveAlerts() []*HealingAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	alerts := make([]*HealingAlert, 0, len(m.activeAlerts))
	for _, alert := range m.activeAlerts {
		alerts = append(alerts, alert)
	}
	return alerts
}

// GetAlertHistory returns the alert history
func (m *HealingAlertManager) GetAlertHistory() []*HealingAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := make([]*HealingAlert, len(m.alertHistory))
	copy(history, m.alertHistory)
	return history
}

// Start starts the alert manager's periodic evaluation
func (m *HealingAlertManager) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return fmt.Errorf("alert manager already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// Start evaluation loop
	m.wg.Add(1)
	go m.evaluationLoop()

	return nil
}

// Stop stops the alert manager
func (m *HealingAlertManager) Stop() {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return
	}

	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

// IsRunning returns true if the alert manager is running
func (m *HealingAlertManager) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

// evaluationLoop periodically evaluates alert rules
func (m *HealingAlertManager) evaluationLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.EvaluationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.metricsMu.RLock()
			metrics := m.lastMetrics
			m.metricsMu.RUnlock()

			m.EvaluateRules(metrics)

		case <-m.ctx.Done():
			return
		}
	}
}

// ClearAlerts clears all active alerts (useful for testing)
func (m *HealingAlertManager) ClearAlerts() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activeAlerts = make(map[string]*HealingAlert)
	m.lastAlertTime = make(map[string]time.Time)
}
