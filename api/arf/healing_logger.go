package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogFormat represents the output format for logs
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// LogConfig configures the healing logger
type LogConfig struct {
	Level             LogLevel
	Format            LogFormat
	Output            io.Writer
	BufferSize        int
	IncludeStackTrace bool
}

// LogFields represents structured log fields
type LogFields map[string]interface{}

// HealingError represents an error with additional context
type HealingError struct {
	Message string
	Code    string
	Cause   error
}

func (e *HealingError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// HealingLogger provides structured logging for healing workflows
type HealingLogger struct {
	config   *LogConfig
	logger   *log.Logger
	fields   LogFields
	mu       sync.RWMutex
	buffer   chan logEntry
	stopChan chan struct{}
}

type logEntry struct {
	Level     LogLevel
	Message   string
	Fields    LogFields
	Timestamp time.Time
	Error     error
}

// NewHealingLogger creates a new healing logger
func NewHealingLogger(config *LogConfig) *HealingLogger {
	if config == nil {
		config = &LogConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
			Output: os.Stdout,
		}
	}

	if config.Output == nil {
		config.Output = os.Stdout
	}

	if config.BufferSize == 0 {
		config.BufferSize = 100
	}

	hl := &HealingLogger{
		config:   config,
		logger:   log.New(config.Output, "", 0),
		fields:   make(LogFields),
		buffer:   make(chan logEntry, config.BufferSize),
		stopChan: make(chan struct{}),
	}

	// Start the log writer goroutine
	go hl.writer()

	return hl
}

// writer processes log entries from the buffer
func (hl *HealingLogger) writer() {
	for {
		select {
		case entry := <-hl.buffer:
			hl.writeLog(entry)
		case <-hl.stopChan:
			// Flush remaining logs
			for len(hl.buffer) > 0 {
				entry := <-hl.buffer
				hl.writeLog(entry)
			}
			return
		}
	}
}

// Flush waits for all buffered logs to be written
func (hl *HealingLogger) Flush() {
	// Wait for buffer to empty
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case <-timeout:
			return
		default:
			if len(hl.buffer) == 0 {
				// Give writer goroutine a chance to finish
				time.Sleep(1 * time.Millisecond)
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}
}

// writeLog writes a single log entry
func (hl *HealingLogger) writeLog(entry logEntry) {
	// Check log level (lower values = higher priority, so reverse the check)
	if entry.Level < hl.config.Level {
		return
	}

	// Merge fields
	fields := make(LogFields)

	// Only merge entry fields if they exist
	if entry.Fields != nil {
		for k, v := range entry.Fields {
			fields[k] = v
		}
	}

	// Add standard fields
	fields["timestamp"] = entry.Timestamp.Format(time.RFC3339)
	fields["level"] = entry.Level.String()
	fields["message"] = entry.Message

	// Add error details if present
	if entry.Error != nil {
		fields["error"] = entry.Error.Error()
		if healErr, ok := entry.Error.(*HealingError); ok {
			fields["error_code"] = healErr.Code
			if healErr.Cause != nil {
				fields["error_cause"] = healErr.Cause.Error()
			}
		}

		// Add stack trace if configured and error level
		if hl.config.IncludeStackTrace && entry.Level == LogLevelError {
			fields["stack_trace"] = getStackTrace()
		}
	}

	// Format and write
	switch hl.config.Format {
	case LogFormatJSON:
		hl.writeJSON(fields)
	case LogFormatText:
		hl.writeText(fields)
	}
}

// writeJSON writes a log entry in JSON format
func (hl *HealingLogger) writeJSON(fields LogFields) {
	data, err := json.Marshal(fields)
	if err != nil {
		// Fallback to text on JSON error
		hl.logger.Printf("Failed to marshal log to JSON: %v", err)
		return
	}
	hl.logger.Println(string(data))
}

// writeText writes a log entry in text format
func (hl *HealingLogger) writeText(fields LogFields) {
	// Format: TIMESTAMP [LEVEL] message key1=value1 key2=value2
	parts := []string{
		fields["timestamp"].(string),
		fmt.Sprintf("[%s]", fields["level"]),
		fields["message"].(string),
	}

	// Add other fields in sorted order for consistency
	var fieldParts []string
	for k, v := range fields {
		if k != "timestamp" && k != "level" && k != "message" {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	if len(fieldParts) > 0 {
		parts = append(parts, fieldParts...)
	}

	hl.logger.Println(strings.Join(parts, " "))
}

// WithFields creates a new logger with additional fields
func (hl *HealingLogger) WithFields(fields LogFields) *HealingLogger {
	hl.mu.Lock()
	defer hl.mu.Unlock()

	// Merge fields into current logger's fields
	for k, v := range fields {
		hl.fields[k] = v
	}

	return hl
}

// Log methods

func (hl *HealingLogger) Debug(message string) {
	hl.logWithFields(LogLevelDebug, message, nil)
}

func (hl *HealingLogger) Info(message string) {
	hl.logWithFields(LogLevelInfo, message, nil)
}

func (hl *HealingLogger) Warn(message string) {
	hl.logWithFields(LogLevelWarn, message, nil)
}

func (hl *HealingLogger) Error(message string, err error) {
	hl.logWithFields(LogLevelError, message, err)
}

func (hl *HealingLogger) logWithFields(level LogLevel, message string, err error) {
	hl.mu.RLock()
	fields := make(LogFields)
	for k, v := range hl.fields {
		fields[k] = v
	}
	hl.mu.RUnlock()
	hl.log(level, message, fields, err)
}

func (hl *HealingLogger) log(level LogLevel, message string, fields LogFields, err error) {
	entry := logEntry{
		Level:     level,
		Message:   message,
		Fields:    fields,
		Timestamp: time.Now(),
		Error:     err,
	}

	select {
	case hl.buffer <- entry:
	default:
		// Buffer full, write directly (may block)
		hl.writeLog(entry)
	}
}

// Healing-specific logging methods

// LogHealingStarted logs the start of a healing attempt
func (hl *HealingLogger) LogHealingStarted(ctx context.Context, transformID, attemptPath, triggerReason string, errors []string) {
	fields := LogFields{
		"transformation_id": transformID,
		"attempt_path":      attemptPath,
		"trigger_reason":    triggerReason,
		"error_count":       len(errors),
		"errors":            errors,
	}
	hl.log(LogLevelInfo, "Healing started", fields, nil)
}

// LogHealingCompleted logs the completion of a healing attempt
func (hl *HealingLogger) LogHealingCompleted(ctx context.Context, transformID, attemptPath, result string, duration time.Duration) {
	fields := LogFields{
		"transformation_id": transformID,
		"attempt_path":      attemptPath,
		"result":            result,
		"duration_ms":       duration.Milliseconds(),
	}
	hl.log(LogLevelInfo, "Healing completed", fields, nil)
}

// LogHealingFailed logs a failed healing attempt
func (hl *HealingLogger) LogHealingFailed(ctx context.Context, transformID, attemptPath string, err error) {
	fields := LogFields{
		"transformation_id": transformID,
		"attempt_path":      attemptPath,
	}
	hl.log(LogLevelError, "Healing failed", fields, err)
}

// LogLLMAnalysis logs LLM error analysis
func (hl *HealingLogger) LogLLMAnalysis(ctx context.Context, transformID, attemptPath string, analysis *LLMAnalysisResult) {
	fields := LogFields{
		"transformation_id": transformID,
		"attempt_path":      attemptPath,
		"error_type":        analysis.ErrorType,
		"confidence":        analysis.Confidence,
		"suggested_fix":     analysis.SuggestedFix,
		"risk_assessment":   analysis.RiskAssessment,
	}
	hl.log(LogLevelInfo, "LLM analysis completed", fields, nil)
}

// LogChildSpawned logs when a child healing attempt is created
func (hl *HealingLogger) LogChildSpawned(ctx context.Context, transformID, parentPath, childPath, reason string) {
	fields := LogFields{
		"transformation_id": transformID,
		"parent_path":       parentPath,
		"child_path":        childPath,
		"spawn_reason":      reason,
	}
	hl.log(LogLevelInfo, "Child healing spawned", fields, nil)
}

// LogCircuitBreakerTrip logs circuit breaker state changes
func (hl *HealingLogger) LogCircuitBreakerTrip(transformID, state string, failures int, cooldown time.Duration) {
	fields := LogFields{
		"transformation_id":    transformID,
		"circuit_state":        state,
		"consecutive_failures": failures,
		"cooldown_seconds":     cooldown.Seconds(),
	}
	hl.log(LogLevelWarn, "Circuit breaker tripped", fields, nil)
}

// LogLLMCost logs LLM usage and costs
func (hl *HealingLogger) LogLLMCost(transformID, model string, inputTokens, outputTokens int, cost float64, cacheHit bool) {
	fields := LogFields{
		"transformation_id": transformID,
		"model":             model,
		"input_tokens":      inputTokens,
		"output_tokens":     outputTokens,
		"total_tokens":      inputTokens + outputTokens,
		"cost":              cost,
		"cache_hit":         cacheHit,
	}
	level := LogLevelInfo
	if cost > 1.0 { // Warn for expensive operations
		level = LogLevelWarn
	}
	hl.log(level, "LLM usage recorded", fields, nil)
}

// LogBudgetAlert logs budget threshold alerts
func (hl *HealingLogger) LogBudgetAlert(alert BudgetAlert) {
	fields := LogFields{
		"alert_type":  alert.Type,
		"current":     alert.Current,
		"limit":       alert.Limit,
		"percentage":  alert.Percentage,
		"recommended": alert.Recommended,
	}
	hl.log(LogLevelWarn, alert.Message, fields, nil)
}

// LogPerformanceMetrics logs healing performance metrics
func (hl *HealingLogger) LogPerformanceMetrics(metrics HealingCoordinatorMetrics) {
	fields := LogFields{
		"total_attempts":      metrics.CompletedTasks,
		"success_rate":        metrics.SuccessRate,
		"average_duration_ms": metrics.AverageHealingDuration.Milliseconds(),
		"circuit_state":       metrics.CircuitBreakerState,
		"llm_calls":           metrics.TotalLLMCalls,
		"llm_cost":            metrics.TotalLLMCost,
		"cache_hit_rate":      metrics.LLMCacheHitRate,
	}
	hl.log(LogLevelInfo, "Performance metrics update", fields, nil)
}

// Close gracefully shuts down the logger
func (hl *HealingLogger) Close() {
	close(hl.stopChan)
}

// getStackTrace returns the current stack trace
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// Global logger instance (can be replaced with custom implementation)
var globalHealingLogger *HealingLogger

func init() {
	// Initialize with default config
	globalHealingLogger = NewHealingLogger(nil)
}

// GetHealingLogger returns the global healing logger
func GetHealingLogger() *HealingLogger {
	return globalHealingLogger
}

// SetHealingLogger sets a custom global healing logger
func SetHealingLogger(logger *HealingLogger) {
	if globalHealingLogger != nil {
		globalHealingLogger.Close()
	}
	globalHealingLogger = logger
}
