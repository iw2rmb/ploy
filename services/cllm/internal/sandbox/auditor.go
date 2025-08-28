package sandbox

import (
	"sync"
	"time"
)

// AuditLog represents a single audit log entry
type AuditLog struct {
	Timestamp time.Time `json:"timestamp"`
	Operation string    `json:"operation"`
	Details   string    `json:"details"`
	Result    string    `json:"result"`
	UserID    string    `json:"user_id,omitempty"`
}

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	Severity  string    `json:"severity"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	Source    string    `json:"source"`
}

// ResourceUsage represents current resource consumption
type ResourceUsage struct {
	MemoryUsed int64 `json:"memory_used"`
	DiskUsed   int64 `json:"disk_used"`
	CPUTime    int64 `json:"cpu_time"`
	Processes  int   `json:"processes"`
}

// ResourceViolation represents a resource limit violation
type ResourceViolation struct {
	Resource  string `json:"resource"`
	Current   int64  `json:"current"`
	Limit     int64  `json:"limit"`
	Timestamp time.Time `json:"timestamp"`
}

// SecurityAuditor interface defines audit logging capabilities
type SecurityAuditor interface {
	// Audit logging
	LogOperation(operation, details, result string)
	GetAuditLogs() []AuditLog
	
	// Security event logging
	LogSecurityEvent(eventType, severity, action, details, source string)
	GetSecurityEvents() []SecurityEvent
	
	// Clear logs (for testing)
	ClearLogs()
}

// ResourceMonitor interface defines resource monitoring capabilities
type ResourceMonitor interface {
	GetCurrentUsage() *ResourceUsage
	CheckResourceLimits() (bool, []ResourceViolation)
	Stop()
}

// DefaultSecurityAuditor implements the SecurityAuditor interface
type DefaultSecurityAuditor struct {
	mu             sync.RWMutex
	auditLogs      []AuditLog
	securityEvents []SecurityEvent
	maxLogEntries  int
}

// NewSecurityAuditor creates a new security auditor
func NewSecurityAuditor(maxLogEntries int) SecurityAuditor {
	return &DefaultSecurityAuditor{
		auditLogs:      make([]AuditLog, 0),
		securityEvents: make([]SecurityEvent, 0),
		maxLogEntries:  maxLogEntries,
	}
}

// LogOperation logs an operational audit event
func (a *DefaultSecurityAuditor) LogOperation(operation, details, result string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	log := AuditLog{
		Timestamp: time.Now(),
		Operation: operation,
		Details:   details,
		Result:    result,
	}
	
	a.auditLogs = append(a.auditLogs, log)
	
	// Trim logs if they exceed max entries
	if len(a.auditLogs) > a.maxLogEntries {
		a.auditLogs = a.auditLogs[len(a.auditLogs)-a.maxLogEntries:]
	}
}

// GetAuditLogs returns a copy of all audit logs
func (a *DefaultSecurityAuditor) GetAuditLogs() []AuditLog {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	logs := make([]AuditLog, len(a.auditLogs))
	copy(logs, a.auditLogs)
	return logs
}

// LogSecurityEvent logs a security-related event
func (a *DefaultSecurityAuditor) LogSecurityEvent(eventType, severity, action, details, source string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	event := SecurityEvent{
		Timestamp: time.Now(),
		EventType: eventType,
		Severity:  severity,
		Action:    action,
		Details:   details,
		Source:    source,
	}
	
	a.securityEvents = append(a.securityEvents, event)
	
	// Trim events if they exceed max entries
	if len(a.securityEvents) > a.maxLogEntries {
		a.securityEvents = a.securityEvents[len(a.securityEvents)-a.maxLogEntries:]
	}
}

// GetSecurityEvents returns a copy of all security events
func (a *DefaultSecurityAuditor) GetSecurityEvents() []SecurityEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	events := make([]SecurityEvent, len(a.securityEvents))
	copy(events, a.securityEvents)
	return events
}

// ClearLogs clears all logged entries (for testing)
func (a *DefaultSecurityAuditor) ClearLogs() {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.auditLogs = make([]AuditLog, 0)
	a.securityEvents = make([]SecurityEvent, 0)
}

// DefaultResourceMonitor implements the ResourceMonitor interface
type DefaultResourceMonitor struct {
	mu             sync.RWMutex
	stopped        bool
	manager        *Manager
	maxMemory      int64
	maxDiskUsage   int64
	maxCPUTime     int64
	maxProcesses   int
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor(manager *Manager) *DefaultResourceMonitor {
	return &DefaultResourceMonitor{
		manager:        manager,
		maxMemory:      1024 * 1024 * 1024, // Default 1GB
		maxDiskUsage:   5 * 1024 * 1024 * 1024, // Default 5GB
		maxCPUTime:     300, // Default 5 minutes
		maxProcesses:   20,  // Default 20 processes
	}
}

// GetCurrentUsage returns current resource usage statistics
func (m *DefaultResourceMonitor) GetCurrentUsage() *ResourceUsage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.stopped {
		return &ResourceUsage{}
	}
	
	// In a real implementation, this would gather actual system metrics
	// For now, we return mock data that demonstrates the interface
	return &ResourceUsage{
		MemoryUsed: 50 * 1024 * 1024, // 50MB
		DiskUsed:   10 * 1024 * 1024, // 10MB
		CPUTime:    30,               // 30 seconds
		Processes:  3,                // 3 processes
	}
}

// CheckResourceLimits checks if current usage is within configured limits
func (m *DefaultResourceMonitor) CheckResourceLimits() (bool, []ResourceViolation) {
	usage := m.GetCurrentUsage()
	violations := make([]ResourceViolation, 0)
	
	now := time.Now()
	
	// Check memory limit
	if usage.MemoryUsed > m.maxMemory {
		violations = append(violations, ResourceViolation{
			Resource:  "memory",
			Current:   usage.MemoryUsed,
			Limit:     m.maxMemory,
			Timestamp: now,
		})
	}
	
	// Check disk usage limit
	if usage.DiskUsed > m.maxDiskUsage {
		violations = append(violations, ResourceViolation{
			Resource:  "disk",
			Current:   usage.DiskUsed,
			Limit:     m.maxDiskUsage,
			Timestamp: now,
		})
	}
	
	// Check CPU time limit
	if usage.CPUTime > m.maxCPUTime {
		violations = append(violations, ResourceViolation{
			Resource:  "cpu_time",
			Current:   usage.CPUTime,
			Limit:     m.maxCPUTime,
			Timestamp: now,
		})
	}
	
	// Check process count limit
	if usage.Processes > m.maxProcesses {
		violations = append(violations, ResourceViolation{
			Resource:  "processes",
			Current:   int64(usage.Processes),
			Limit:     int64(m.maxProcesses),
			Timestamp: now,
		})
	}
	
	return len(violations) == 0, violations
}

// Stop stops the resource monitor
func (m *DefaultResourceMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.stopped = true
}