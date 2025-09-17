package cleanup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTTLCleanupServiceIdentifyPreviewJobs ensures preview job metadata is parsed correctly
func TestTTLCleanupServiceIdentifyPreviewJobs(t *testing.T) {
	jobs := []NomadJob{
		{ID: "app-one-1234567", SubmitTime: time.Now().Add(-2 * time.Hour).UnixNano()},
		{ID: "invalid-job", SubmitTime: time.Now().Add(-30 * time.Minute).UnixNano()},
		{ID: "another-app-abcdef1234567890", SubmitTime: time.Now().Add(-3 * time.Hour).UnixNano()},
	}

	svc := &TTLCleanupService{}
	preview := svc.identifyPreviewJobs(jobs)

	if len(preview) != 2 {
		t.Fatalf("expected 2 preview jobs, got %d", len(preview))
	}

	if preview[0].App != "app-one" {
		t.Fatalf("expected first preview app 'app-one', got %s", preview[0].App)
	}
	if preview[1].SHA != "abcdef1234567890" {
		t.Fatalf("expected SHA to be parsed correctly, got %s", preview[1].SHA)
	}
}

// TestTTLCleanupServiceDetermineJobsToClean verifies TTL and max age enforcement
func TestTTLCleanupServiceDetermineJobsToClean(t *testing.T) {
	svc := &TTLCleanupService{config: &TTLConfig{
		PreviewTTL: 2 * time.Hour,
		MaxAge:     6 * time.Hour,
	}}

	jobs := []PreviewJobInfo{
		{JobName: "recent", Age: 30 * time.Minute},
		{JobName: "expiredByTTL", Age: 3 * time.Hour},
		{JobName: "expiredByMax", Age: 8 * time.Hour},
	}

	toClean := svc.determineJobsToClean(jobs)

	if len(toClean) != 2 {
		t.Fatalf("expected 2 jobs to clean, got %d", len(toClean))
	}

	if toClean[0].JobName != "expiredByTTL" {
		t.Fatalf("expected first job expiredByTTL, got %s", toClean[0].JobName)
	}
	if toClean[0].Reason == "" || toClean[1].Reason == "" {
		t.Fatalf("expected cleanup reasons to be provided: %#v", toClean)
	}
	if !strings.Contains(toClean[0].Reason, "preview TTL") {
		t.Fatalf("expected TTL reason, got %s", toClean[0].Reason)
	}
	if !strings.Contains(toClean[1].Reason, "maximum age") {
		t.Fatalf("expected max age reason, got %s", toClean[1].Reason)
	}
}

// TestTTLCleanupServiceCleanupJobDryRunSkipsNomadCall ensures dry-run does not execute the nomad command
func TestTTLCleanupServiceCleanupJobDryRunSkipsNomadCall(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "nomad_invoked")
	writeFakeNomad(t, tmp, fmt.Sprintf("#!/bin/sh\necho ran > %s\n", shellQuote(marker)))
	t.Setenv("PATH", fmt.Sprintf("%s:%s", tmp, os.Getenv("PATH")))

	svc := &TTLCleanupService{config: &TTLConfig{DryRun: true}}

	if err := svc.cleanupJob(PreviewJobInfo{JobName: "example"}); err != nil {
		t.Fatalf("cleanupJob returned error: %v", err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("expected nomad command not to be invoked in dry run mode")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error if marker missing: %v", err)
	}
}

// TestTTLCleanupServiceCleanupJobTreatsNotFoundAsSuccess ensures not found output is treated as success
func TestTTLCleanupServiceCleanupJobTreatsNotFoundAsSuccess(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "not_found_marker")
	fakeNomad := fmt.Sprintf("#!/bin/sh\necho 'job not found' 1>&2\necho \"$@\" > %s\nexit 1\n", shellQuote(marker))
	writeFakeNomad(t, tmp, fakeNomad)
	t.Setenv("PATH", fmt.Sprintf("%s:%s", tmp, os.Getenv("PATH")))

	svc := &TTLCleanupService{config: &TTLConfig{DryRun: false}}
	err := svc.cleanupJob(PreviewJobInfo{JobName: "preview-job"})
	if err != nil {
		t.Fatalf("expected not found to be treated as success, got error: %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected marker to indicate command executed: %v", err)
	}
}

// TestTTLCleanupServiceCleanupJobReturnsErrorOnFailure ensures general failures bubble up
func TestTTLCleanupServiceCleanupJobReturnsErrorOnFailure(t *testing.T) {
	tmp := t.TempDir()
	fakeNomad := "#!/bin/sh\necho 'permission denied' 1>&2\nexit 1\n"
	writeFakeNomad(t, tmp, fakeNomad)
	t.Setenv("PATH", fmt.Sprintf("%s:%s", tmp, os.Getenv("PATH")))

	svc := &TTLCleanupService{config: &TTLConfig{DryRun: false}}
	err := svc.cleanupJob(PreviewJobInfo{JobName: "preview-job"})
	if err == nil {
		t.Fatalf("expected error when nomad command fails without not found message")
	}
}

// writeFakeNomad writes a fake nomad binary into dir with provided script body
func writeFakeNomad(t *testing.T, dir, script string) {
	path := filepath.Join(dir, "nomad")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake nomad: %v", err)
	}
}

// shellQuote wraps a path for safe insertion into shell script
func shellQuote(path string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(path, "'", "'\\''"))
}
