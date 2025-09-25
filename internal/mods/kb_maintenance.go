package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/nats-io/nats.go"
)

// MaintenanceConfig contains configuration for KB maintenance jobs
type MaintenanceConfig struct {
	// Scheduling configuration
	CompactionInterval     time.Duration `json:"compaction_interval"`      // 24h
	SummaryRebuildInterval time.Duration `json:"summary_rebuild_interval"` // 6h
	SnapshotInterval       time.Duration `json:"snapshot_interval"`        // 1h

	// Job execution settings
	JobTimeout        time.Duration `json:"job_timeout"`         // 2h
	MaxConcurrentJobs int           `json:"max_concurrent_jobs"` // 3
	JobRetryCount     int           `json:"job_retry_count"`     // 2

	// Resource limits
	JobMemoryLimit string `json:"job_memory_limit"` // "512M"
	JobCPULimit    string `json:"job_cpu_limit"`    // "500m"

	// Feature flags
	EnableCompactionJobs   bool `json:"enable_compaction_jobs"`   // true
	EnableSummaryJobs      bool `json:"enable_summary_jobs"`      // true
	EnableSnapshotJobs     bool `json:"enable_snapshot_jobs"`     // true
	EnableMetricsReporting bool `json:"enable_metrics_reporting"` // true

	// Job submission configuration
	NomadJobTemplate string `json:"nomad_job_template"` // Path to HCL template
	JobNamespace     string `json:"job_namespace"`      // "kb-maintenance"
	JobPriority      int    `json:"job_priority"`       // 25
}

// DefaultMaintenanceConfig returns reasonable defaults for maintenance jobs
func DefaultMaintenanceConfig() *MaintenanceConfig {
	return &MaintenanceConfig{
		CompactionInterval:     24 * time.Hour,
		SummaryRebuildInterval: 6 * time.Hour,
		SnapshotInterval:       1 * time.Hour,
		JobTimeout:             2 * time.Hour,
		MaxConcurrentJobs:      3,
		JobRetryCount:          2,
		JobMemoryLimit:         "512M",
		JobCPULimit:            "500m",
		EnableCompactionJobs:   true,
		EnableSummaryJobs:      true,
		EnableSnapshotJobs:     true,
		EnableMetricsReporting: true,
		NomadJobTemplate:       "/opt/templates/kb-maintenance.hcl",
		JobNamespace:           "kb-maintenance",
		JobPriority:            25,
	}
}

// MaintenanceJob represents a KB maintenance job submission
type MaintenanceJob struct {
	ID          string                 `json:"id"`
	Type        MaintenanceJobType     `json:"type"`
	Parameters  map[string]interface{} `json:"parameters"`
	SubmittedAt time.Time              `json:"submitted_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Status      JobStatus              `json:"status"`
	Stats       *DeduplicationStats    `json:"stats,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// MaintenanceJobType represents different types of maintenance jobs
type MaintenanceJobType string

const (
	CompactionJobType         MaintenanceJobType = "compaction"
	SummaryRebuildJobType     MaintenanceJobType = "summary_rebuild"
	SnapshotJobType           MaintenanceJobType = "snapshot"
	PatchDeduplicationJobType MaintenanceJobType = "patch_deduplication"
)

// JobStatus represents the status of a maintenance job
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// MaintenanceScheduler handles scheduling and execution of KB maintenance jobs
type MaintenanceScheduler struct {
	storage         KBStorage
	sigGenerator    EnhancedSignatureGenerator
	lockMgr         KBLockManager
	summaryComputer *SummaryComputer
	// jobSubmitter would be added when orchestration interface is extended
	config *MaintenanceConfig

	// Internal state
	activeJobs map[string]*MaintenanceJob
	lastRun    map[MaintenanceJobType]time.Time

	// Event subscription for JetStream lock events
	conn         *nats.Conn
	js           nats.JetStreamContext
	subscription *nats.Subscription
}

// NewMaintenanceScheduler creates a new maintenance scheduler
func NewMaintenanceScheduler(
	storage KBStorage,
	sigGenerator EnhancedSignatureGenerator,
	lockMgr KBLockManager,
	summaryComputer *SummaryComputer,
	config *MaintenanceConfig,
) *MaintenanceScheduler {
	if config == nil {
		config = DefaultMaintenanceConfig()
	}

	scheduler := &MaintenanceScheduler{
		storage:         storage,
		sigGenerator:    sigGenerator,
		lockMgr:         lockMgr,
		summaryComputer: summaryComputer,
		config:          config,
		activeJobs:      make(map[string]*MaintenanceJob),
		lastRun:         make(map[MaintenanceJobType]time.Time),
	}

	if conn, js, err := initJetstreamConnection(); err != nil {
		fmt.Printf("Warning: failed to initialize JetStream for maintenance events: %v\n", err)
	} else {
		scheduler.conn = conn
		scheduler.js = js
	}

	return scheduler
}

// StartScheduler begins the maintenance job scheduling loop
func (ms *MaintenanceScheduler) StartScheduler(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	// Subscribe to lock release events if JetStream is available
	if ms.js != nil {
		if err := ms.subscribeLockEvents(ctx); err != nil {
			fmt.Printf("Warning: failed to subscribe to lock events: %v\n", err)
		}
	}

	defer func() {
		if ms.subscription != nil {
			_ = ms.subscription.Unsubscribe()
		}
		if ms.conn != nil {
			ms.conn.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ms.scheduleJobs(ctx); err != nil {
				// Log error but continue scheduling
				fmt.Printf("Error scheduling maintenance jobs: %v\n", err)
			}
		}
	}
}

// scheduleJobs checks if any maintenance jobs need to be scheduled
func (ms *MaintenanceScheduler) scheduleJobs(ctx context.Context) error {
	now := time.Now()

	// Check compaction jobs
	if ms.config.EnableCompactionJobs {
		lastCompaction := ms.lastRun[CompactionJobType]
		if now.Sub(lastCompaction) >= ms.config.CompactionInterval {
			if err := ms.scheduleCompactionJob(ctx); err != nil {
				return fmt.Errorf("failed to schedule compaction job: %w", err)
			}
			ms.lastRun[CompactionJobType] = now
		}
	}

	// Check summary rebuild jobs
	if ms.config.EnableSummaryJobs {
		lastSummary := ms.lastRun[SummaryRebuildJobType]
		if now.Sub(lastSummary) >= ms.config.SummaryRebuildInterval {
			if err := ms.scheduleSummaryRebuildJob(ctx); err != nil {
				return fmt.Errorf("failed to schedule summary rebuild job: %w", err)
			}
			ms.lastRun[SummaryRebuildJobType] = now
		}
	}

	// Check snapshot jobs
	if ms.config.EnableSnapshotJobs {
		lastSnapshot := ms.lastRun[SnapshotJobType]
		if now.Sub(lastSnapshot) >= ms.config.SnapshotInterval {
			if err := ms.scheduleSnapshotJob(ctx); err != nil {
				return fmt.Errorf("failed to schedule snapshot job: %w", err)
			}
			ms.lastRun[SnapshotJobType] = now
		}
	}

	return nil
}

// scheduleCompactionJob submits a new compaction job
func (ms *MaintenanceScheduler) scheduleCompactionJob(ctx context.Context) error {
	if len(ms.activeJobs) >= ms.config.MaxConcurrentJobs {
		return fmt.Errorf("max concurrent jobs reached (%d)", ms.config.MaxConcurrentJobs)
	}

	jobID := fmt.Sprintf("compaction-%d", time.Now().Unix())
	job := &MaintenanceJob{
		ID:   jobID,
		Type: CompactionJobType,
		Parameters: map[string]interface{}{
			"dry_run":         false,
			"full_compaction": true,
		},
		SubmittedAt: time.Now(),
		Status:      JobStatusPending,
	}

	// Submit job via Nomad
	err := ms.submitNomadJob(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to submit compaction job: %w", err)
	}

	ms.activeJobs[jobID] = job
	return nil
}

// scheduleSummaryRebuildJob submits a summary rebuild job
func (ms *MaintenanceScheduler) scheduleSummaryRebuildJob(ctx context.Context) error {
	if len(ms.activeJobs) >= ms.config.MaxConcurrentJobs {
		return fmt.Errorf("max concurrent jobs reached (%d)", ms.config.MaxConcurrentJobs)
	}

	jobID := fmt.Sprintf("summary-rebuild-%d", time.Now().Unix())
	job := &MaintenanceJob{
		ID:   jobID,
		Type: SummaryRebuildJobType,
		Parameters: map[string]interface{}{
			"rebuild_all":  true,
			"force_update": false,
		},
		SubmittedAt: time.Now(),
		Status:      JobStatusPending,
	}

	err := ms.submitNomadJob(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to submit summary rebuild job: %w", err)
	}

	ms.activeJobs[jobID] = job
	return nil
}

// scheduleSnapshotJob submits a snapshot generation job
func (ms *MaintenanceScheduler) scheduleSnapshotJob(ctx context.Context) error {
	if len(ms.activeJobs) >= ms.config.MaxConcurrentJobs {
		return fmt.Errorf("max concurrent jobs reached (%d)", ms.config.MaxConcurrentJobs)
	}

	jobID := fmt.Sprintf("snapshot-%d", time.Now().Unix())
	job := &MaintenanceJob{
		ID:   jobID,
		Type: SnapshotJobType,
		Parameters: map[string]interface{}{
			"include_metrics": ms.config.EnableMetricsReporting,
		},
		SubmittedAt: time.Now(),
		Status:      JobStatusPending,
	}

	err := ms.submitNomadJob(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to submit snapshot job: %w", err)
	}

	ms.activeJobs[jobID] = job
	return nil
}

// submitNomadJob submits a maintenance job to Nomad
func (ms *MaintenanceScheduler) submitNomadJob(ctx context.Context, job *MaintenanceJob) error {
	// Prepare job parameters
	jobVars := map[string]interface{}{
		"job_id":       job.ID,
		"job_type":     string(job.Type),
		"parameters":   job.Parameters,
		"memory_limit": ms.config.JobMemoryLimit,
		"cpu_limit":    ms.config.JobCPULimit,
		"timeout":      ms.config.JobTimeout.String(),
		"namespace":    ms.config.JobNamespace,
		"priority":     ms.config.JobPriority,
		"retry_count":  ms.config.JobRetryCount,
	}

	// Convert to JSON for job template
	jobVarsJSON, err := json.Marshal(jobVars)
	if err != nil {
		return fmt.Errorf("failed to marshal job variables: %w", err)
	}

	// Submit job using orchestration interface
	// For now, this is a simplified interface - the actual implementation
	// would need to integrate with the existing job submission infrastructure
	return ms.submitJobViaOrchestration(ctx, string(jobVarsJSON))
}

// submitJobViaOrchestration submits job through the orchestration layer
func (ms *MaintenanceScheduler) submitJobViaOrchestration(ctx context.Context, jobConfig string) error {
	// This would integrate with the existing orchestration.Submit functions
	// For now, we'll implement a placeholder that demonstrates the pattern

	// In a real implementation, this would:
	// 1. Load the HCL job template from ms.config.NomadJobTemplate
	// 2. Substitute variables using orchestration.RenderTemplate
	// 3. Submit using orchestration.SubmitAndWaitTerminal

	return nil // Placeholder implementation
}

// MonitorJobs checks the status of active maintenance jobs
func (ms *MaintenanceScheduler) MonitorJobs(ctx context.Context) error {
	for jobID, job := range ms.activeJobs {
		// Check job status from Nomad
		status, err := ms.getJobStatus(ctx, jobID)
		if err != nil {
			continue // Skip jobs we can't check
		}

		// Update job status
		job.Status = status

		// Handle completed/failed jobs
		if status == JobStatusCompleted || status == JobStatusFailed {
			now := time.Now()
			job.CompletedAt = &now

			// Get job results if completed successfully
			if status == JobStatusCompleted {
				stats, err := ms.getJobResults(ctx, jobID)
				if err == nil {
					job.Stats = stats
				}
			}

			// Remove from active jobs
			delete(ms.activeJobs, jobID)

			// Store job record for history/metrics
			if ms.config.EnableMetricsReporting {
				ms.recordJobMetrics(job)
			}
		}
	}

	return nil
}

// getJobStatus queries Nomad for job status
func (ms *MaintenanceScheduler) getJobStatus(ctx context.Context, jobID string) (JobStatus, error) {
	// This would query the Nomad API to get the actual job status
	// For now, return a placeholder
	return JobStatusRunning, nil
}

// getJobResults retrieves results from a completed job
func (ms *MaintenanceScheduler) getJobResults(ctx context.Context, jobID string) (*DeduplicationStats, error) {
	// This would retrieve job artifacts/results from Nomad
	// The job would write results to a known location that we can read
	return &DeduplicationStats{}, nil
}

// recordJobMetrics records job completion metrics
func (ms *MaintenanceScheduler) recordJobMetrics(job *MaintenanceJob) {
	// This would record metrics to whatever monitoring system is in use
	// For now, just log the completion
	fmt.Printf("Maintenance job %s (%s) completed with status %s\n",
		job.ID, job.Type, job.Status)
}

// GetActiveJobs returns currently active maintenance jobs
func (ms *MaintenanceScheduler) GetActiveJobs() []*MaintenanceJob {
	jobs := make([]*MaintenanceJob, 0, len(ms.activeJobs))
	for _, job := range ms.activeJobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// CancelJob cancels an active maintenance job
func (ms *MaintenanceScheduler) CancelJob(ctx context.Context, jobID string) error {
	job, exists := ms.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	// Cancel job in Nomad
	err := ms.cancelNomadJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to cancel job: %w", err)
	}

	// Update local state
	job.Status = JobStatusCancelled
	now := time.Now()
	job.CompletedAt = &now

	delete(ms.activeJobs, jobID)
	return nil
}

// cancelNomadJob cancels a job in Nomad
func (ms *MaintenanceScheduler) cancelNomadJob(ctx context.Context, jobID string) error {
	// This would use the Nomad API to stop/purge the job
	// For now, return success
	return nil
}

// TriggerManualCompaction manually triggers a compaction job
func (ms *MaintenanceScheduler) TriggerManualCompaction(ctx context.Context, params map[string]interface{}) (*MaintenanceJob, error) {
	jobID := fmt.Sprintf("manual-compaction-%d", time.Now().Unix())
	job := &MaintenanceJob{
		ID:          jobID,
		Type:        CompactionJobType,
		Parameters:  params,
		SubmittedAt: time.Now(),
		Status:      JobStatusPending,
	}

	err := ms.submitNomadJob(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to submit manual compaction: %w", err)
	}

	ms.activeJobs[jobID] = job
	return job, nil
}

// GetJobHistory returns historical job information
func (ms *MaintenanceScheduler) GetJobHistory(ctx context.Context, jobType MaintenanceJobType, limit int) ([]*MaintenanceJob, error) {
	// This would query a persistent store (likely the KB itself) for job history
	// For now, return empty list
	return []*MaintenanceJob{}, nil
}

// MaintenanceStatus provides overall maintenance system status
type MaintenanceStatus struct {
	ActiveJobs     int                              `json:"active_jobs"`
	LastRun        map[MaintenanceJobType]time.Time `json:"last_run"`
	NextScheduled  map[MaintenanceJobType]time.Time `json:"next_scheduled"`
	TotalJobsToday map[MaintenanceJobType]int       `json:"total_jobs_today"`
	SystemHealth   string                           `json:"system_health"`
	ConfigVersion  string                           `json:"config_version"`
}

// GetMaintenanceStatus returns current maintenance system status
func (ms *MaintenanceScheduler) GetMaintenanceStatus(ctx context.Context) (*MaintenanceStatus, error) {
	status := &MaintenanceStatus{
		ActiveJobs:     len(ms.activeJobs),
		LastRun:        ms.lastRun,
		NextScheduled:  make(map[MaintenanceJobType]time.Time),
		TotalJobsToday: make(map[MaintenanceJobType]int),
		SystemHealth:   "healthy",
		ConfigVersion:  "1.0.0",
	}

	// Calculate next scheduled times
	if ms.config.EnableCompactionJobs {
		lastCompaction := ms.lastRun[CompactionJobType]
		status.NextScheduled[CompactionJobType] = lastCompaction.Add(ms.config.CompactionInterval)
	}

	if ms.config.EnableSummaryJobs {
		lastSummary := ms.lastRun[SummaryRebuildJobType]
		status.NextScheduled[SummaryRebuildJobType] = lastSummary.Add(ms.config.SummaryRebuildInterval)
	}

	if ms.config.EnableSnapshotJobs {
		lastSnapshot := ms.lastRun[SnapshotJobType]
		status.NextScheduled[SnapshotJobType] = lastSnapshot.Add(ms.config.SnapshotInterval)
	}

	// System health would be determined by recent job success rates, storage health, etc.
	if len(ms.activeJobs) > ms.config.MaxConcurrentJobs {
		status.SystemHealth = "degraded"
	}

	return status, nil
}

// initJetstreamConnection initializes a JetStream connection for events
func initJetstreamConnection() (*nats.Conn, nats.JetStreamContext, error) {
	url := utils.Getenv("PLOY_JETSTREAM_URL", "")
	if url == "" {
		url = utils.Getenv("NATS_ADDR", nats.DefaultURL)
	}
	if url == "" {
		return nil, nil, fmt.Errorf("jetstream url not configured")
	}

	opts := []nats.Option{nats.Name("ploy-maintenance-scheduler")}
	if creds := utils.Getenv("PLOY_JETSTREAM_CREDS", ""); creds != "" {
		opts = append(opts, nats.UserCredentials(creds))
	}
	user := utils.Getenv("PLOY_JETSTREAM_USER", "")
	if user != "" {
		opts = append(opts, nats.UserInfo(user, utils.Getenv("PLOY_JETSTREAM_PASSWORD", "")))
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	return conn, js, nil
}

// subscribeLockEvents subscribes to lock release events to trigger maintenance jobs
func (ms *MaintenanceScheduler) subscribeLockEvents(ctx context.Context) error {
	// Subscribe to all lock release events
	subject := "mods.kb.lock.released.*"

	sub, err := ms.conn.Subscribe(subject, func(msg *nats.Msg) {
		ms.handleLockEvent(ctx, msg)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to lock events: %w", err)
	}

	ms.subscription = sub
	fmt.Printf("Subscribed to KB lock events on subject: %s\n", subject)
	return nil
}

// handleLockEvent processes lock release events and triggers appropriate maintenance jobs
func (ms *MaintenanceScheduler) handleLockEvent(ctx context.Context, msg *nats.Msg) {
	var eventData map[string]interface{}
	if err := json.Unmarshal(msg.Data, &eventData); err != nil {
		fmt.Printf("Error unmarshaling lock event: %v\n", err)
		return
	}

	// Extract the lock key from the event
	kbID, hasKBID := eventData["kb_id"].(string)
	key, hasKey := eventData["key"].(string)
	if !hasKBID && hasKey {
		kbID = strings.TrimPrefix(key, "writers/")
	}
	if kbID == "" {
		fmt.Printf("Error: lock event missing kb_id/key field\n")
		return
	}

	fmt.Printf("Received lock release event for key: %s\n", kbID)

	// Trigger appropriate maintenance jobs based on the lock key
	if err := ms.triggerMaintenanceForKey(ctx, kbID); err != nil {
		fmt.Printf("Error triggering maintenance for key %s: %v\n", kbID, err)
	}
}

// triggerMaintenanceForKey triggers maintenance jobs based on the released lock key
func (ms *MaintenanceScheduler) triggerMaintenanceForKey(ctx context.Context, key string) error {
	// Determine what type of maintenance is needed based on the lock key pattern

	// Pattern: lang/signature -> trigger summary rebuild for that signature
	if strings.Contains(key, "/") && !strings.HasPrefix(key, "maintenance/") {
		parts := strings.Split(key, "/")
		if len(parts) >= 2 {
			// This looks like a signature lock (lang/signature)
			if ms.config.EnableSummaryJobs {
				return ms.triggerSignatureSummaryRebuild(ctx, parts[0], parts[1])
			}
		}
	}

	// Pattern: maintenance/* -> already handled by regular scheduling
	if strings.HasPrefix(key, "maintenance/") {
		// Don't trigger additional maintenance for maintenance locks
		return nil
	}

	// For other patterns, trigger general maintenance
	if ms.config.EnableCompactionJobs && ms.shouldTriggerCompaction() {
		return ms.scheduleCompactionJob(ctx)
	}

	return nil
}

// triggerSignatureSummaryRebuild triggers a summary rebuild for a specific signature
func (ms *MaintenanceScheduler) triggerSignatureSummaryRebuild(ctx context.Context, lang, signature string) error {
	if len(ms.activeJobs) >= ms.config.MaxConcurrentJobs {
		return fmt.Errorf("max concurrent jobs reached (%d)", ms.config.MaxConcurrentJobs)
	}

	jobID := fmt.Sprintf("event-summary-rebuild-%s-%s-%d", lang, signature, time.Now().Unix())
	job := &MaintenanceJob{
		ID:   jobID,
		Type: SummaryRebuildJobType,
		Parameters: map[string]interface{}{
			"language":     lang,
			"signature":    signature,
			"force_update": true,
			"triggered_by": "lock_release_event",
		},
		SubmittedAt: time.Now(),
		Status:      JobStatusPending,
	}

	err := ms.submitNomadJob(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to submit event-triggered summary rebuild: %w", err)
	}

	ms.activeJobs[jobID] = job
	fmt.Printf("Triggered summary rebuild job %s for %s/%s\n", jobID, lang, signature)
	return nil
}

// shouldTriggerCompaction determines if compaction should be triggered
func (ms *MaintenanceScheduler) shouldTriggerCompaction() bool {
	lastCompaction := ms.lastRun[CompactionJobType]
	minInterval := time.Hour // Don't trigger compaction more than once per hour via events
	return time.Since(lastCompaction) >= minInterval
}
