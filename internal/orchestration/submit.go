package orchestration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/internal/utils"
)

// Concurrency limiter for direct Nomad SDK submissions.
var submitSlots chan struct{}

func init() {
	// Default 4 concurrent submissions; configurable via NOMAD_SUBMIT_MAX_CONCURRENCY
	n := envInt("NOMAD_SUBMIT_MAX_CONCURRENCY", 4)
	if n < 1 {
		n = 1
	}
	submitSlots = make(chan struct{}, n)
}

func acquireSubmit() { submitSlots <- struct{}{} }
func releaseSubmit() { <-submitSlots }

// Detects whether to use the platform Nomad Job Manager wrapper on VPS.
func useJobManager() bool {
	// Allow explicit override via env
	if v := os.Getenv("USE_NOMAD_JOB_MANAGER"); v != "" {
		return v != "0" && strings.ToLower(v) != "false"
	}
	if _, err := os.Stat("/opt/hashicorp/bin/nomad-job-manager.sh"); err == nil {
		return true
	}
	return false
}

func jobManagerPath() string {
	if p := os.Getenv("NOMAD_JOB_MANAGER"); p != "" {
		return p
	}
	return "/opt/hashicorp/bin/nomad-job-manager.sh"
}

// Extract job name from HCL file by looking for: job "name" {
func extractJobName(hclPath string) (string, error) {
	data, err := os.ReadFile(hclPath)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?m)^\s*job\s+"([^"]+)"\s*{`)
	m := re.FindSubmatch(data)
	if len(m) >= 2 {
		return string(m[1]), nil
	}
	return "", errors.New("job name not found in HCL")
}

// AllocationStatusLite mirrors minimal fields from Nomad allocation JSON used for terminal checks
type AllocationStatusLite struct {
	ID           string `json:"ID"`
	ClientStatus string `json:"ClientStatus"`
	TaskStates   map[string]struct {
		Events []struct {
			Type    string            `json:"Type"`
			Details map[string]string `json:"Details"`
		} `json:"Events"`
	} `json:"TaskStates"`
}

// Submit HCL using the job manager wrapper
func submitWithJobManager(hclPath string) (string, error) {
	name, err := extractJobName(hclPath)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(jobManagerPath(), "run", "--job", name, "--file", hclPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nomad-job-manager run failed: %v: %s", err, out.String())
	}
	return name, nil
}

// Wait for terminal state using job manager outputs
func waitTerminalWithJobManager(jobName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	allocAppearGuard := envDur("NOMAD_ALLOC_APPEARANCE_TIMEOUT", 90*time.Second)
	sawAnyAllocs := false
	for time.Now().Before(deadline) {
		// Get allocations as JSON (parse stdout only; wrapper emits logs)
		cmd := exec.Command(jobManagerPath(), "allocs", "--job", jobName, "--format", "json")
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		raw := stdout.Bytes()
		// The wrapper may prepend log lines; find JSON start
		if idx := bytes.IndexByte(raw, '['); idx >= 0 {
			raw = raw[idx:]
		}
		var allocs []AllocationStatusLite
		if err := json.Unmarshal(raw, &allocs); err == nil {
			// Terminal if any alloc failed or any alloc completed
			sawRunningOrPending := false
			if len(allocs) > 0 {
				sawAnyAllocs = true
			}
			for _, a := range allocs {
				// Prefer explicit terminated event exit codes when available
				for _, ts := range a.TaskStates {
					// Scan events from latest to earliest
					for i := len(ts.Events) - 1; i >= 0; i-- {
						ev := ts.Events[i]
						if strings.EqualFold(ev.Type, "Terminated") {
							if code, ok := ev.Details["exit_code"]; ok {
								if code == "0" {
									return nil
								}
								return fmt.Errorf("job %s allocation failed (exit %s)", jobName, code)
							}
						}
					}
				}
				switch strings.ToLower(a.ClientStatus) {
				case "failed":
					return fmt.Errorf("job %s allocation failed (%s)", jobName, a.ID)
				case "complete", "completed":
					return nil
				case "running", "pending", "starting":
					sawRunningOrPending = true
				}
			}
			if !sawRunningOrPending && len(allocs) > 0 {
				// Not clearly terminal; wait
			}
		}
		// Fast-fail guard: no allocations appeared within guard window
		if !sawAnyAllocs && time.Since(deadline.Add(-timeout)) > allocAppearGuard {
			return fmt.Errorf("no allocations created for job %s within %s (check job evaluations/constraints)", jobName, allocAppearGuard)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for job %s to complete", jobName)
}

// Submit reads an HCL job file, parses and registers it via Nomad API.
func Submit(jobPath string) error {
	if useJobManager() {
		_, err := submitWithJobManager(jobPath)
		return err
	}
	acquireSubmit()
	defer releaseSubmit()
	hcl, err := os.ReadFile(jobPath)
	if err != nil {
		return fmt.Errorf("read job file: %w", err)
	}
	client, err := newNomadClient()
	if err != nil {
		return err
	}
	jobs := client.Jobs()
	job, err := jobs.ParseHCL(string(hcl), true)
	if err != nil {
		return fmt.Errorf("parse HCL: %w", err)
	}
	_, _, err = jobs.Register(job, nil)
	if err != nil {
		return fmt.Errorf("register job: %w", err)
	}
	return nil
}

// SubmitAndWaitHealthy parses, registers the job, and waits for min healthy allocations.
func SubmitAndWaitHealthy(jobPath string, expectedCount int, timeout time.Duration) error {
	if useJobManager() {
		name, err := submitWithJobManager(jobPath)
		if err != nil {
			return err
		}
		// Healthy == at least one running alloc within timeout
		monitor := NewHealthMonitor()
		return monitor.WaitForHealthyAllocations(name, expectedCount, timeout)
	}
	acquireSubmit()
	defer releaseSubmit()
	hcl, err := os.ReadFile(jobPath)
	if err != nil {
		return fmt.Errorf("read job file: %w", err)
	}
	client, err := newNomadClient()
	if err != nil {
		return err
	}
	jobs := client.Jobs()
	job, err := jobs.ParseHCL(string(hcl), true)
	if err != nil {
		return fmt.Errorf("parse HCL: %w", err)
	}
	if _, _, err := jobs.Register(job, nil); err != nil {
		return fmt.Errorf("register job: %w", err)
	}
	name := ""
	if job != nil && job.Name != nil {
		name = *job.Name
	}
	if name == "" {
		return nil
	}
	monitor := NewHealthMonitor()
	return monitor.WaitForHealthyAllocations(name, expectedCount, timeout)
}

// ValidateJob parses HCL to validate syntax; returns error if invalid.
func ValidateJob(jobPath string) error {
	if useJobManager() {
		// For wrapper path, rely on nomad CLI to validate HCL by converting (-output)
		cmd := exec.Command("nomad", "job", "run", "-output", jobPath)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("job parse/validate failed via nomad CLI: %w", err)
		}
		return nil
	}
	// Validation does not register jobs, but still talks to API
	acquireSubmit()
	defer releaseSubmit()
	hcl, err := os.ReadFile(jobPath)
	if err != nil {
		return err
	}
	client, err := newNomadClient()
	if err != nil {
		return err
	}
	if _, err := client.Jobs().ParseHCL(string(hcl), true); err != nil {
		return fmt.Errorf("job parse/validate failed: %w", err)
	}
	return nil
}

// PlanJob is not implemented in SDK mode; returns a placeholder message.
func PlanJob(jobPath string) (string, error) {
	return "plan not implemented in SDK mode", nil
}

// StreamJobLogs is not implemented without Nomad client log streaming; returns error.
func StreamJobLogs(jobID string, follow bool) error {
	return fmt.Errorf("log streaming not implemented in orchestration SDK mode")
}

// WaitHealthy waits until a job has at least one healthy allocation or timeout.
func WaitHealthy(jobName string, timeout time.Duration) error {
	monitor := NewHealthMonitor()
	return monitor.WaitForHealthyAllocations(jobName, 1, timeout)
}

// SubmitAndWaitTerminal registers a batch job and waits until its allocations reach a terminal state
// (complete or failed), or the timeout elapses. This is intended for short-lived planner/reducer jobs.
func SubmitAndWaitTerminal(jobPath string, timeout time.Duration) error {
	start := time.Now()
	allocAppearGuard := envDur("NOMAD_ALLOC_APPEARANCE_TIMEOUT", 90*time.Second)
	if useJobManager() {
		name, err := submitWithJobManager(jobPath)
		if err != nil {
			return err
		}
		// Wait with guard: if no allocations appear within guard, fail fast
		if err := waitTerminalWithJobManager(name, timeout); err != nil {
			return err
		}
		// Note: waitTerminalWithJobManager handles alloc checks internally; guard is less applicable here
		_ = start
		_ = allocAppearGuard
		return nil
	}
	acquireSubmit()
	defer releaseSubmit()
	hcl, err := os.ReadFile(jobPath)
	if err != nil {
		return fmt.Errorf("read job file: %w", err)
	}
	client, err := newNomadClient()
	if err != nil {
		return err
	}
	jobs := client.Jobs()
	job, err := jobs.ParseHCL(string(hcl), true)
	if err != nil {
		return fmt.Errorf("parse HCL: %w", err)
	}
	if _, _, err := jobs.Register(job, nil); err != nil {
		return fmt.Errorf("register job: %w", err)
	}
	name := ""
	if job != nil && job.Name != nil {
		name = *job.Name
	}
	if name == "" {
		return fmt.Errorf("job name not resolved")
	}

	deadline := time.Now().Add(timeout)
	var lastIndex uint64
	waitTime := envDur("NOMAD_BLOCKING_WAIT", 30*time.Second)
	sawAnyAllocs := false
	for time.Now().Before(deadline) {
		// Use blocking query to reduce control-plane load
		q := &nomadapi.QueryOptions{WaitIndex: lastIndex, WaitTime: waitTime, AllowStale: true}
		allocs, meta, err := client.Jobs().Allocations(name, false, q)
		if err != nil {
			// Non-fatal: backoff briefly and retry
			time.Sleep(2 * time.Second)
			continue
		}
		if meta != nil && meta.LastIndex > 0 {
			lastIndex = meta.LastIndex
		}
		// Evaluate terminal states
		sawRunning := false
		for _, a := range allocs {
			sawAnyAllocs = true
			cs := a.ClientStatus
			if cs == "complete" {
				return nil
			}
			if cs == "failed" {
				return fmt.Errorf("job %s allocation failed (%s)", name, a.ID)
			}
			if cs == "running" || cs == "pending" || cs == "starting" {
				sawRunning = true
			}
		}
		if !sawAnyAllocs && time.Since(start) > allocAppearGuard {
			return fmt.Errorf("no allocations created for job %s within %s (check job evaluations/constraints)", name, allocAppearGuard)
		}
		if !sawRunning && len(allocs) > 0 {
			// Allocations exist but none running and none complete/failed; give them time
		}
		// Do not sleep here; blocking query already waited. Loop continues.
	}
	return fmt.Errorf("timeout waiting for job %s to complete", name)
}

func newNomadClient() (*nomadapi.Client, error) {
	cfg := nomadapi.DefaultConfig()
	if addr := utils.Getenv("NOMAD_ADDR", ""); addr != "" {
		cfg.Address = addr
	}
	// Install retrying HTTP client for resilience outside VPS wrapper
	if cfg.HttpClient == nil {
		cfg.HttpClient = &http.Client{
			Transport: NewDefaultRetryTransport(nil),
			Timeout:   60 * time.Second,
		}
	} else {
		// Wrap existing transport if present
		if cfg.HttpClient.Transport == nil {
			cfg.HttpClient.Transport = NewDefaultRetryTransport(nil)
		} else {
			cfg.HttpClient.Transport = &RetryTransport{Base: cfg.HttpClient.Transport, MaxRetries: envInt("NOMAD_HTTP_MAX_RETRIES", 5), BaseDelay: envDur("NOMAD_HTTP_BASE_DELAY", 500*time.Millisecond), MaxDelay: envDur("NOMAD_HTTP_MAX_DELAY", 30*time.Second), JitterFrac: 0.2}
		}
		if cfg.HttpClient.Timeout == 0 {
			cfg.HttpClient.Timeout = 60 * time.Second
		}
	}
	return nomadapi.NewClient(cfg)
}

// DeregisterJob deregisters a job by name; if purge is true, allocations are purged
func DeregisterJob(jobName string, purge bool) error {
	client, err := newNomadClient()
	if err != nil {
		return err
	}
	_, _, err = client.Jobs().Deregister(jobName, purge, nil)
	if err != nil {
		return fmt.Errorf("deregister job: %w", err)
	}
	return nil
}
