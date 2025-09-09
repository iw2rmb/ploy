//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHealingLogger_LogLevels(t *testing.T) {
	tests := []struct {
		name          string
		configLevel   LogLevel
		logMethod     func(logger *HealingLogger)
		shouldLog     bool
		expectedLevel string
	}{
		{
			name:        "debug level allows debug",
			configLevel: LogLevelDebug,
			logMethod: func(logger *HealingLogger) {
				logger.Debug("test debug message")
			},
			shouldLog:     true,
			expectedLevel: "DEBUG",
		},
		{
			name:        "info level blocks debug",
			configLevel: LogLevelInfo,
			logMethod: func(logger *HealingLogger) {
				logger.Debug("test debug message")
			},
			shouldLog: false,
		},
		{
			name:        "info level allows info",
			configLevel: LogLevelInfo,
			logMethod: func(logger *HealingLogger) {
				logger.Info("test info message")
			},
			shouldLog:     true,
			expectedLevel: "INFO",
		},
		{
			name:        "error level only allows error",
			configLevel: LogLevelError,
			logMethod: func(logger *HealingLogger) {
				logger.Warn("test warn message")
			},
			shouldLog: false,
		},
		{
			name:        "error level allows error",
			configLevel: LogLevelError,
			logMethod: func(logger *HealingLogger) {
				logger.Error("test error message", nil)
			},
			shouldLog:     true,
			expectedLevel: "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			config := &LogConfig{
				Level:  tt.configLevel,
				Format: LogFormatJSON,
				Output: &buf,
			}

			logger := NewHealingLogger(config)
			tt.logMethod(logger)
			logger.Flush() // Ensure log is written

			output := buf.String()
			if tt.shouldLog {
				if output == "" {
					t.Errorf("Expected log output but got none")
				}
				if !strings.Contains(output, tt.expectedLevel) {
					t.Errorf("Expected log level %s in output: %s", tt.expectedLevel, output)
				}
			} else {
				if output != "" {
					t.Errorf("Expected no log output but got: %s", output)
				}
			}
		})
	}
}

func TestHealingLogger_StructuredFields(t *testing.T) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelDebug,
		Format: LogFormatJSON,
		Output: &buf,
	}

	logger := NewHealingLogger(config)
	logger.WithFields(LogFields{
		"transformation_id": "test-123",
		"attempt_path":      "1.2.3",
		"depth":             3,
	}).Info("Healing attempt started")
	logger.Flush()

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	// Check required fields
	if logEntry["transformation_id"] != "test-123" {
		t.Errorf("Expected transformation_id=test-123, got %v", logEntry["transformation_id"])
	}
	if logEntry["attempt_path"] != "1.2.3" {
		t.Errorf("Expected attempt_path=1.2.3, got %v", logEntry["attempt_path"])
	}
	if logEntry["message"] != "Healing attempt started" {
		t.Errorf("Expected message='Healing attempt started', got %v", logEntry["message"])
	}
}

func TestHealingLogger_HealingLifecycle(t *testing.T) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelInfo,
		Format: LogFormatJSON,
		Output: &buf,
	}

	logger := NewHealingLogger(config)
	ctx := context.Background()

	// Log healing started
	logger.LogHealingStarted(ctx, "transform-1", "1.1", "build_failure", []string{"compilation error"})

	// Log LLM analysis
	logger.LogLLMAnalysis(ctx, "transform-1", "1.1", &LLMAnalysisResult{
		ErrorType:    "compilation",
		Confidence:   0.85,
		SuggestedFix: "Add missing import",
	})

	// Log child spawned
	logger.LogChildSpawned(ctx, "transform-1", "1.1", "1.1.1", "new_error_discovered")

	// Log healing completed
	logger.LogHealingCompleted(ctx, "transform-1", "1.1", "success", 5*time.Second)
	logger.Flush()

	// Verify all events were logged
	logs := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(logs) != 4 {
		t.Errorf("Expected 4 log entries, got %d", len(logs))
	}

	// Verify first log entry
	var firstLog map[string]interface{}
	if err := json.Unmarshal([]byte(logs[0]), &firstLog); err != nil {
		t.Fatalf("Failed to parse first log: %v", err)
	}
	if !strings.Contains(firstLog["message"].(string), "Healing started") {
		t.Errorf("First log should be healing started")
	}
}

func TestHealingLogger_CircuitBreaker(t *testing.T) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelWarn,
		Format: LogFormatJSON,
		Output: &buf,
	}

	logger := NewHealingLogger(config)

	// Log circuit breaker trip
	logger.LogCircuitBreakerTrip("transform-1", "open", 5, 30*time.Second)
	logger.Flush()

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["level"] != "WARN" {
		t.Errorf("Circuit breaker trip should be WARN level")
	}
	if logEntry["transformation_id"] != "transform-1" {
		t.Errorf("Expected transformation_id in circuit breaker log")
	}
	if logEntry["consecutive_failures"] != float64(5) {
		t.Errorf("Expected consecutive_failures=5, got %v", logEntry["consecutive_failures"])
	}
}

func TestHealingLogger_LLMCostTracking(t *testing.T) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelInfo,
		Format: LogFormatJSON,
		Output: &buf,
	}

	logger := NewHealingLogger(config)

	// Log LLM cost event
	logger.LogLLMCost("transform-1", "gpt-4", 1000, 500, 0.06, true)
	logger.Flush()

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["model"] != "gpt-4" {
		t.Errorf("Expected model=gpt-4, got %v", logEntry["model"])
	}
	if logEntry["input_tokens"] != float64(1000) {
		t.Errorf("Expected input_tokens=1000, got %v", logEntry["input_tokens"])
	}
	if logEntry["cost"] != 0.06 {
		t.Errorf("Expected cost=0.06, got %v", logEntry["cost"])
	}
	if logEntry["cache_hit"] != true {
		t.Errorf("Expected cache_hit=true, got %v", logEntry["cache_hit"])
	}
}

func TestHealingLogger_ConcurrentLogging(t *testing.T) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelInfo,
		Format: LogFormatJSON,
		Output: &buf,
	}

	// Simulate concurrent logging
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Create a new logger instance for each goroutine to avoid field conflicts
			localLogger := NewHealingLogger(config)
			localLogger.WithFields(LogFields{
				"goroutine": id,
			}).Info("Concurrent log message")
			localLogger.Flush()
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()

	// Verify all logs were written
	logs := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(logs) != 10 {
		t.Errorf("Expected 10 log entries, got %d", len(logs))
	}

	// Verify each log is valid JSON
	for i, log := range logs {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(log), &entry); err != nil {
			t.Errorf("Log %d is not valid JSON: %v", i, err)
		}
	}
}

func TestHealingLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelInfo,
		Format: LogFormatText,
		Output: &buf,
	}

	logger := NewHealingLogger(config)
	logger.WithFields(LogFields{
		"transformation_id": "test-123",
	}).Info("Test message")
	logger.Flush()

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Errorf("Text format should contain log level, got: %s", output)
	}
	if !strings.Contains(output, "Test message") {
		t.Errorf("Text format should contain message, got: %s", output)
	}
	if !strings.Contains(output, "transformation_id=test-123") {
		t.Errorf("Text format should contain fields, got: %s", output)
	}
}

func TestHealingLogger_ErrorWithStackTrace(t *testing.T) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:             LogLevelError,
		Format:            LogFormatJSON,
		Output:            &buf,
		IncludeStackTrace: true,
	}

	logger := NewHealingLogger(config)
	testErr := &HealingError{
		Message: "Test error",
		Code:    "TEST_ERROR",
	}

	logger.Error("Operation failed", testErr)
	logger.Flush()

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["error"] != "Test error" {
		t.Errorf("Expected error message in log")
	}
	if logEntry["error_code"] != "TEST_ERROR" {
		t.Errorf("Expected error code in log")
	}
	if _, ok := logEntry["stack_trace"]; !ok && config.IncludeStackTrace {
		t.Errorf("Expected stack trace in error log")
	}
}

func BenchmarkHealingLogger_JSONFormat(b *testing.B) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelInfo,
		Format: LogFormatJSON,
		Output: &buf,
	}

	logger := NewHealingLogger(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.WithFields(LogFields{
			"transformation_id": "bench-123",
			"iteration":         i,
		}).Info("Benchmark log message")
		buf.Reset() // Reset buffer to prevent memory growth
	}
}

func BenchmarkHealingLogger_TextFormat(b *testing.B) {
	var buf bytes.Buffer
	config := &LogConfig{
		Level:  LogLevelInfo,
		Format: LogFormatText,
		Output: &buf,
	}

	logger := NewHealingLogger(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.WithFields(LogFields{
			"transformation_id": "bench-123",
			"iteration":         i,
		}).Info("Benchmark log message")
		buf.Reset()
	}
}
