//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealingAlert_Structure(t *testing.T) {
	alert := &HealingAlert{
		Type:     "failure_rate",
		Severity: "critical",
		Message:  "Healing failure rate exceeds 80%",
		Details: map[string]interface{}{
			"failure_rate": 0.85,
			"window":       "1h",
		},
		Timestamp: time.Now(),
		Resolved:  false,
	}

	assert.Equal(t, "failure_rate", alert.Type)
	assert.Equal(t, "critical", alert.Severity)
	assert.False(t, alert.Resolved)
	assert.NotNil(t, alert.Details)
}

func TestHealingAlertManager_Initialization(t *testing.T) {
	config := &AlertConfig{
		Enabled:              true,
		FailureRateThreshold: 0.8,
		MaxTreeDepth:         8,
		MaxDuration:          4 * time.Hour,
		EvaluationInterval:   1 * time.Minute,
	}

	manager := NewHealingAlertManager(config)
	require.NotNil(t, manager)
	assert.Equal(t, config, manager.GetConfig())
	assert.Empty(t, manager.GetActiveAlerts())
}

func TestHealingAlertManager_FailureRateAlert(t *testing.T) {
	config := &AlertConfig{
		Enabled:              true,
		FailureRateThreshold: 0.5, // Lower threshold to ensure trigger
		EvaluationInterval:   100 * time.Millisecond,
	}

	manager := NewHealingAlertManager(config)

	// Create metrics with high failure rate
	metrics := HealingCoordinatorMetrics{
		CompletedTasks: 20,
		FailedTasks:    80,
		SuccessRate:    0.2, // 20% success = 80% failure
	}

	// Register alert callback
	alertReceived := false
	var receivedAlert *HealingAlert
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		alertReceived = true
		receivedAlert = alert
	})

	// Evaluate rules
	manager.EvaluateRules(metrics)

	// Wait for async callback
	time.Sleep(100 * time.Millisecond)

	// Should trigger alert
	assert.True(t, alertReceived)
	require.NotNil(t, receivedAlert)
	assert.Equal(t, "failure_rate", receivedAlert.Type)
	assert.Equal(t, "critical", receivedAlert.Severity)
	assert.Contains(t, receivedAlert.Message, "80.0%")
}

func TestHealingAlertManager_DeepHierarchyAlert(t *testing.T) {
	config := &AlertConfig{
		Enabled:      true,
		MaxTreeDepth: 8,
	}

	manager := NewHealingAlertManager(config)

	// Register callback
	alertReceived := false
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		if alert.Type == "deep_hierarchy" {
			alertReceived = true
		}
	})

	// Create metrics with deep tree
	metrics := HealingCoordinatorMetrics{}

	// Trigger deep hierarchy detection
	manager.RecordTreeDepth("transform-1", 10) // Depth 10 > threshold 8

	// Evaluate rules
	manager.EvaluateRules(metrics)

	// Wait for async callback
	time.Sleep(100 * time.Millisecond)

	assert.True(t, alertReceived, "Should receive deep hierarchy alert")
}

func TestHealingAlertManager_LongRunningAlert(t *testing.T) {
	config := &AlertConfig{
		Enabled:     true,
		MaxDuration: 100 * time.Millisecond, // Short for testing
	}

	manager := NewHealingAlertManager(config)

	// Start tracking a transformation
	manager.StartTracking("transform-1")

	// Wait for it to exceed duration
	time.Sleep(150 * time.Millisecond)

	// Register callback
	alertReceived := false
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		if alert.Type == "long_running" {
			alertReceived = true
		}
	})

	// Evaluate rules
	manager.EvaluateRules(HealingCoordinatorMetrics{})

	// Wait for async callback
	time.Sleep(100 * time.Millisecond)

	assert.True(t, alertReceived, "Should receive long running alert")

	// Stop tracking
	manager.StopTracking("transform-1")

	// Alert should be resolved
	alerts := manager.GetActiveAlerts()
	for _, alert := range alerts {
		if alert.Type == "long_running" {
			assert.True(t, alert.Resolved, "Alert should be resolved after stopping")
		}
	}
}

func TestHealingAlertManager_AlertDeduplication(t *testing.T) {
	config := &AlertConfig{
		Enabled:              true,
		FailureRateThreshold: 0.8,
		DeduplicationWindow:  5 * time.Second,
	}

	manager := NewHealingAlertManager(config)

	alertCount := 0
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		if alert.Type == "failure_rate" {
			alertCount++
		}
	})

	// High failure metrics
	metrics := HealingCoordinatorMetrics{
		CompletedTasks: 10,
		FailedTasks:    90,
		SuccessRate:    0.1,
	}

	// Evaluate multiple times quickly
	for i := 0; i < 5; i++ {
		manager.EvaluateRules(metrics)
		time.Sleep(10 * time.Millisecond)
	}

	// Should only receive one alert due to deduplication
	assert.Equal(t, 1, alertCount, "Should deduplicate alerts within window")
}

func TestHealingAlertManager_AlertResolution(t *testing.T) {
	config := &AlertConfig{
		Enabled:              true,
		FailureRateThreshold: 0.8,
	}

	manager := NewHealingAlertManager(config)

	// Track resolution
	alertResolved := false
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		if alert.Type == "failure_rate" && alert.Resolved {
			alertResolved = true
		}
	})

	// High failure rate
	badMetrics := HealingCoordinatorMetrics{
		CompletedTasks: 10,
		FailedTasks:    90,
		SuccessRate:    0.1,
	}
	manager.EvaluateRules(badMetrics)

	// Verify alert is active
	assert.Len(t, manager.GetActiveAlerts(), 1)

	// Good metrics (low failure rate)
	goodMetrics := HealingCoordinatorMetrics{
		CompletedTasks: 90,
		FailedTasks:    10,
		SuccessRate:    0.9,
	}
	manager.EvaluateRules(goodMetrics)

	// Wait for async callback
	time.Sleep(100 * time.Millisecond)

	// Alert should be resolved
	assert.True(t, alertResolved, "Should receive resolution alert")

	// No active alerts
	activeAlerts := manager.GetActiveAlerts()
	unresolved := 0
	for _, alert := range activeAlerts {
		if !alert.Resolved {
			unresolved++
		}
	}
	assert.Equal(t, 0, unresolved, "No unresolved alerts should remain")
}

func TestHealingAlertManager_CircuitBreakerAlert(t *testing.T) {
	config := &AlertConfig{
		Enabled: true,
	}

	manager := NewHealingAlertManager(config)

	alertReceived := false
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		if alert.Type == "circuit_breaker" {
			alertReceived = true
			assert.Equal(t, "warning", alert.Severity)
		}
	})

	// Metrics with open circuit breaker
	metrics := HealingCoordinatorMetrics{
		CircuitBreakerState: "open",
		ConsecutiveFailures: 5,
	}

	manager.EvaluateRules(metrics)

	// Wait for async callback
	time.Sleep(100 * time.Millisecond)

	assert.True(t, alertReceived, "Should receive circuit breaker alert")
}

func TestHealingAlertManager_MultipleCallbacks(t *testing.T) {
	config := &AlertConfig{
		Enabled:              true,
		FailureRateThreshold: 0.8,
	}

	manager := NewHealingAlertManager(config)

	// Register multiple callbacks
	callback1Called := false
	callback2Called := false
	callback3Called := false

	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		callback1Called = true
	})
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		callback2Called = true
	})
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		callback3Called = true
	})

	// Trigger alert
	metrics := HealingCoordinatorMetrics{
		CompletedTasks: 10,
		FailedTasks:    90,
		SuccessRate:    0.1,
	}
	manager.EvaluateRules(metrics)

	// All callbacks should be called
	time.Sleep(100 * time.Millisecond) // Allow async callbacks to complete
	assert.True(t, callback1Called)
	assert.True(t, callback2Called)
	assert.True(t, callback3Called)
}

func TestHealingAlertManager_AlertHistory(t *testing.T) {
	config := &AlertConfig{
		Enabled:              true,
		FailureRateThreshold: 0.8,
		MaxHistorySize:       10,
	}

	manager := NewHealingAlertManager(config)

	// Generate multiple alerts
	for i := 0; i < 15; i++ {
		metrics := HealingCoordinatorMetrics{
			CompletedTasks: 10,
			FailedTasks:    90,
			SuccessRate:    0.1,
		}
		manager.EvaluateRules(metrics)

		// Clear deduplication by advancing time
		manager.lastAlertTime = make(map[string]time.Time)
		time.Sleep(10 * time.Millisecond)
	}

	// History should be capped at MaxHistorySize
	history := manager.GetAlertHistory()
	assert.LessOrEqual(t, len(history), 10, "History should be capped")
}

func TestHealingAlertManager_ConcurrentAlerts(t *testing.T) {
	config := &AlertConfig{
		Enabled:              true,
		FailureRateThreshold: 0.5,
		MaxTreeDepth:         5,
		MaxDuration:          100 * time.Millisecond,
	}

	manager := NewHealingAlertManager(config)

	// Track alerts received
	var alertCount int32
	manager.RegisterAlertCallback(func(alert *HealingAlert) {
		atomic.AddInt32(&alertCount, 1)
	})

	// Start concurrent operations
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Goroutine 1: Generate failure rate alerts
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				metrics := HealingCoordinatorMetrics{
					CompletedTasks: 40,
					FailedTasks:    60,
					SuccessRate:    0.4,
				}
				manager.EvaluateRules(metrics)
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Goroutine 2: Generate deep hierarchy alerts
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				manager.RecordTreeDepth("transform-"+string(rune(i)), i+6)
				time.Sleep(30 * time.Millisecond)
			}
		}
	}()

	// Goroutine 3: Generate long running alerts
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				transformID := "long-" + string(rune(i))
				manager.StartTracking(transformID)
				time.Sleep(150 * time.Millisecond)
				manager.StopTracking(transformID)
			}
		}
	}()

	// Wait for goroutines
	wg.Wait()

	// Should have received multiple alerts without race conditions
	count := atomic.LoadInt32(&alertCount)
	assert.Greater(t, count, int32(0), "Should have received alerts")
}

func TestAlertConfig_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    *AlertConfig
		expectErr bool
	}{
		{
			name: "valid config",
			config: &AlertConfig{
				Enabled:              true,
				FailureRateThreshold: 0.8,
				MaxTreeDepth:         8,
				MaxDuration:          4 * time.Hour,
				EvaluationInterval:   1 * time.Minute,
			},
			expectErr: false,
		},
		{
			name: "invalid failure threshold",
			config: &AlertConfig{
				Enabled:              true,
				FailureRateThreshold: 1.5, // > 1.0
			},
			expectErr: true,
		},
		{
			name: "invalid tree depth",
			config: &AlertConfig{
				Enabled:      true,
				MaxTreeDepth: -1,
			},
			expectErr: true,
		},
		{
			name: "invalid duration",
			config: &AlertConfig{
				Enabled:     true,
				MaxDuration: -1 * time.Hour,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHealingAlertManager_Start_Stop(t *testing.T) {
	config := &AlertConfig{
		Enabled:            true,
		EvaluationInterval: 50 * time.Millisecond,
	}

	manager := NewHealingAlertManager(config)

	// Start the manager
	ctx, cancel := context.WithCancel(context.Background())
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Should be running
	assert.True(t, manager.IsRunning())

	// Stop the manager
	cancel()
	manager.Stop()

	// Should not be running
	assert.False(t, manager.IsRunning())

	// Starting again should work
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	err = manager.Start(ctx2)
	assert.NoError(t, err)
}
