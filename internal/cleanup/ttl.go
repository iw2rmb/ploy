package cleanup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// TTLConfig configures the TTL cleanup service
type TTLConfig struct {
	PreviewTTL      time.Duration `json:"preview_ttl"`       // How long preview allocations live
	CleanupInterval time.Duration `json:"cleanup_interval"`  // How often to run cleanup
	NomadAddr       string        `json:"nomad_addr"`        // Nomad API address
	DryRun          bool          `json:"dry_run"`           // If true, only log what would be cleaned up
	MaxAge          time.Duration `json:"max_age"`           // Maximum age for any preview allocation
}

// DefaultTTLConfig returns sensible defaults for TTL cleanup
func DefaultTTLConfig() *TTLConfig {
	return &TTLConfig{
		PreviewTTL:      24 * time.Hour,  // Preview allocations live for 24 hours
		CleanupInterval: 6 * time.Hour,   // Cleanup runs every 6 hours
		NomadAddr:       getenv("NOMAD_ADDR", "http://127.0.0.1:4646"),
		DryRun:          false,
		MaxAge:          7 * 24 * time.Hour, // Hard limit: 7 days
	}
}

// TTLCleanupService manages TTL cleanup for preview allocations
type TTLCleanupService struct {
	config  *TTLConfig
	ctx     context.Context
	cancel  context.CancelFunc
	client  *http.Client
	running bool
}

// NewTTLCleanupService creates a new TTL cleanup service
func NewTTLCleanupService(config *TTLConfig) *TTLCleanupService {
	if config == nil {
		config = DefaultTTLConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create HTTP client with custom transport for better IPv4/IPv6 handling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true, // Enable both IPv4 and IPv6
		}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	
	return &TTLCleanupService{
		config:  config,
		ctx:     ctx,
		cancel:  cancel,
		client:  client,
		running: false,
	}
}

// NomadJob represents a Nomad job from the API
type NomadJob struct {
	ID           string `json:"ID"`
	Name         string `json:"Name"`
	Status       string `json:"Status"`
	SubmitTime   int64  `json:"SubmitTime"`
	ModifyTime   int64  `json:"ModifyTime"`
	CreateTime   int64  `json:"CreateTime"`
	Type         string `json:"Type"`
	Priority     int    `json:"Priority"`
	Datacenters  []string `json:"Datacenters"`
}

// PreviewJobInfo contains information about a preview job
type PreviewJobInfo struct {
	JobName     string
	App         string
	SHA         string
	Age         time.Duration
	ShouldClean bool
	Reason      string
}

var previewJobPattern = regexp.MustCompile(`^([a-z0-9-]+)-([a-f0-9]{7,40})$`)

// Start begins the TTL cleanup service
func (s *TTLCleanupService) Start() error {
	if s.running {
		return fmt.Errorf("TTL cleanup service is already running")
	}
	
	// Test Nomad connectivity before starting
	if err := s.testNomadConnectivity(); err != nil {
		log.Printf("Warning: Nomad connectivity test failed: %v", err)
		log.Printf("TTL cleanup service will start but may have connection issues")
	}
	
	s.running = true
	log.Printf("Starting TTL cleanup service (interval: %v, preview TTL: %v, max age: %v)", 
		s.config.CleanupInterval, s.config.PreviewTTL, s.config.MaxAge)
	
	// Run initial cleanup
	if err := s.runCleanup(); err != nil {
		log.Printf("Initial cleanup failed: %v", err)
	}
	
	// Start periodic cleanup
	go s.periodicCleanup()
	
	return nil
}

// testNomadConnectivity tests if we can reach the Nomad API
func (s *TTLCleanupService) testNomadConnectivity() error {
	url := fmt.Sprintf("%s/v1/status/leader", s.config.NomadAddr)
	
	req, err := http.NewRequestWithContext(s.ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create connectivity test request: %w", err)
	}
	
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ploy-ttl-cleanup/1.0")
	
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("connectivity test failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connectivity test returned status %d", resp.StatusCode)
	}
	
	log.Printf("Nomad connectivity test successful (URL: %s)", url)
	return nil
}

// Stop stops the TTL cleanup service
func (s *TTLCleanupService) Stop() error {
	if !s.running {
		return fmt.Errorf("TTL cleanup service is not running")
	}
	
	log.Printf("Stopping TTL cleanup service")
	s.cancel()
	s.running = false
	
	return nil
}

// periodicCleanup runs cleanup at configured intervals
func (s *TTLCleanupService) periodicCleanup() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			log.Printf("TTL cleanup service stopped")
			return
		case <-ticker.C:
			if err := s.runCleanup(); err != nil {
				log.Printf("Periodic cleanup failed: %v", err)
			}
		}
	}
}

// runCleanup performs a single cleanup run
func (s *TTLCleanupService) runCleanup() error {
	log.Printf("Running TTL cleanup (dry_run: %v)", s.config.DryRun)
	
	// Get all Nomad jobs
	jobs, err := s.getNomadJobs()
	if err != nil {
		return fmt.Errorf("failed to get Nomad jobs: %w", err)
	}
	
	// Identify preview jobs
	previewJobs := s.identifyPreviewJobs(jobs)
	
	// Determine which jobs should be cleaned up
	jobsToClean := s.determineJobsToClean(previewJobs)
	
	// Perform cleanup
	cleanedCount := 0
	for _, job := range jobsToClean {
		if err := s.cleanupJob(job); err != nil {
			log.Printf("Failed to cleanup job %s: %v", job.JobName, err)
		} else {
			cleanedCount++
		}
	}
	
	log.Printf("TTL cleanup completed: checked %d jobs, cleaned %d preview allocations", 
		len(previewJobs), cleanedCount)
	
	return nil
}

// getNomadJobs retrieves all jobs from Nomad API
func (s *TTLCleanupService) getNomadJobs() ([]NomadJob, error) {
	url := fmt.Sprintf("%s/v1/jobs", s.config.NomadAddr)
	
	// Create request with context for better timeout handling
	req, err := http.NewRequestWithContext(s.ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add headers to ensure proper handling
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ploy-ttl-cleanup/1.0")
	
	resp, err := s.client.Do(req)
	if err != nil {
		// Enhanced error logging for network connectivity issues
		log.Printf("Nomad API connection failed: %v (URL: %s)", err, url)
		return nil, fmt.Errorf("failed to query Nomad API: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		log.Printf("Nomad API returned non-200 status: %d", resp.StatusCode)
		return nil, fmt.Errorf("Nomad API returned status %d", resp.StatusCode)
	}
	
	var jobs []NomadJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode Nomad jobs: %w", err)
	}
	
	log.Printf("Successfully retrieved %d jobs from Nomad API", len(jobs))
	return jobs, nil
}

// identifyPreviewJobs identifies which jobs are preview jobs
func (s *TTLCleanupService) identifyPreviewJobs(jobs []NomadJob) []PreviewJobInfo {
	var previewJobs []PreviewJobInfo
	
	for _, job := range jobs {
		if matches := previewJobPattern.FindStringSubmatch(job.ID); matches != nil {
			app := matches[1]
			sha := matches[2]
			
			// Calculate age based on SubmitTime (nanoseconds since epoch)
			submitTime := time.Unix(0, job.SubmitTime)
			age := time.Since(submitTime)
			
			previewJobs = append(previewJobs, PreviewJobInfo{
				JobName: job.ID,
				App:     app,
				SHA:     sha,
				Age:     age,
			})
		}
	}
	
	return previewJobs
}

// determineJobsToClean determines which preview jobs should be cleaned up
func (s *TTLCleanupService) determineJobsToClean(previewJobs []PreviewJobInfo) []PreviewJobInfo {
	var jobsToClean []PreviewJobInfo
	
	for _, job := range previewJobs {
		shouldClean := false
		reason := ""
		
		// Check if job exceeds maximum age (hard limit)
		if job.Age > s.config.MaxAge {
			shouldClean = true
			reason = fmt.Sprintf("exceeds maximum age (%v > %v)", job.Age, s.config.MaxAge)
		} else if job.Age > s.config.PreviewTTL {
			// Check if job exceeds preview TTL
			shouldClean = true
			reason = fmt.Sprintf("exceeds preview TTL (%v > %v)", job.Age, s.config.PreviewTTL)
		}
		
		if shouldClean {
			job.ShouldClean = true
			job.Reason = reason
			jobsToClean = append(jobsToClean, job)
			
			log.Printf("Preview job %s (app: %s, sha: %s, age: %v) marked for cleanup: %s", 
				job.JobName, job.App, job.SHA, job.Age.Round(time.Minute), reason)
		}
	}
	
	return jobsToClean
}

// cleanupJob removes a single preview job
func (s *TTLCleanupService) cleanupJob(job PreviewJobInfo) error {
	if s.config.DryRun {
		log.Printf("DRY RUN: Would cleanup preview job %s (reason: %s)", job.JobName, job.Reason)
		return nil
	}
	
	log.Printf("Cleaning up preview job %s (reason: %s)", job.JobName, job.Reason)
	
	// Use nomad CLI to stop and purge the job
	cmd := exec.Command("nomad", "job", "stop", "-purge", job.JobName)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		// Check if the job was already not found (which is fine)
		if strings.Contains(string(output), "not found") {
			log.Printf("Preview job %s was already removed", job.JobName)
			return nil
		}
		return fmt.Errorf("failed to stop job %s: %v, output: %s", job.JobName, err, string(output))
	}
	
	log.Printf("Successfully cleaned up preview job %s", job.JobName)
	return nil
}

// GetStats returns statistics about the cleanup service
func (s *TTLCleanupService) GetStats() (map[string]interface{}, error) {
	jobs, err := s.getNomadJobs()
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs for stats: %w", err)
	}
	
	previewJobs := s.identifyPreviewJobs(jobs)
	jobsToClean := s.determineJobsToClean(previewJobs)
	
	stats := map[string]interface{}{
		"service_running":     s.running,
		"total_jobs":          len(jobs),
		"preview_jobs":        len(previewJobs),
		"jobs_to_clean":       len(jobsToClean),
		"config":             s.config,
		"last_check":         time.Now().UTC(),
	}
	
	// Add age distribution
	ageDistribution := make(map[string]int)
	for _, job := range previewJobs {
		switch {
		case job.Age < time.Hour:
			ageDistribution["< 1h"]++
		case job.Age < 6*time.Hour:
			ageDistribution["1h-6h"]++
		case job.Age < 24*time.Hour:
			ageDistribution["6h-24h"]++
		case job.Age < 7*24*time.Hour:
			ageDistribution["1d-7d"]++
		default:
			ageDistribution["> 7d"]++
		}
	}
	stats["age_distribution"] = ageDistribution
	
	return stats, nil
}

// IsRunning returns whether the cleanup service is currently running
func (s *TTLCleanupService) IsRunning() bool {
	return s.running
}

func getenv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}