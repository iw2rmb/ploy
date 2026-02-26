package runs

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRenderRunReportTextHeadersAndArtifacts(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	preGateID := domaintypes.NewJobID()
	migJobID := domaintypes.NewJobID()

	report := RunReport{
		RunID:   runID,
		MigID:   migID,
		MigName: "java17-upgrade",
		SpecID:  specID,
		Repos: []RepoReport{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/service.git",
				BaseRef:     "main",
				TargetRef:   "ploy/java17",
				Status:      "Running",
				Attempt:     1,
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
			},
		},
		Runs: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/service.git",
				BaseRef:     "main",
				TargetRef:   "ploy/java17",
				Attempt:     1,
				Status:      "Running",
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
				Jobs: []RunJobEntry{
					{
						JobID:      preGateID,
						JobType:    "pre_gate",
						JobImage:   "ghcr.io/acme/pre-gate:1",
						Status:     "Running",
						DurationMs: 2450,
					},
					{
						JobID:      migJobID,
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:1",
						Status:     "Success",
						DurationMs: 3000,
						PatchURL:   "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs?download=true&diff_id=abc",
					},
				},
			},
		},
	}

	baseURL, err := url.Parse("https://example.test")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, TextRenderOptions{EnableOSC8: false, BaseURL: baseURL}); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}

	out := buf.String()
	assertContains(t, out, "Mig:   "+migID.String()+"   | java17-upgrade")
	assertContains(t, out, "Spec:  "+specID.String()+" | Download (https://example.test/v1/migs/"+migID.String()+"/specs/latest)")
	assertContains(t, out, "Repos: 1")
	assertContains(t, out, "Run:   "+runID.String())
	assertContains(t, out, "Repo:  [1/1] github.com/acme/service (https://github.com/acme/service.git) main -> ploy/java17")
	assertContains(t, out, "Artifacts")
	assertNotContains(t, out, "State")
	if strings.Count(out, "Logs (https://example.test/v1/runs/") != 1 {
		t.Fatalf("expected exactly one logs link in output, got: %q", out)
	}
	assertContains(t, out, "Logs (https://example.test/v1/runs/")
	assertContains(t, out, " | Patch (https://example.test/v1/runs/")
	assertContains(t, out, "⣾")
}

func TestRenderRunReportTextExitOneLiners(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	preGateID := domaintypes.NewJobID()
	healID := domaintypes.NewJobID()
	errText := "compile\nfailed at step 2"
	failCode := int32(137)
	healCode := int32(0)

	report := RunReport{
		RunID:   runID,
		MigID:   migID,
		MigName: "healing-run",
		SpecID:  specID,
		Repos: []RepoReport{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/heal.git",
				BaseRef:     "main",
				TargetRef:   "ploy/heal",
				Status:      "Fail",
				Attempt:     1,
				LastError:   &errText,
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
			},
		},
		Runs: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/heal.git",
				BaseRef:     "main",
				TargetRef:   "ploy/heal",
				Attempt:     1,
				Status:      "Fail",
				LastError:   &errText,
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
				Jobs: []RunJobEntry{
					{
						JobID:      preGateID,
						JobType:    "pre_gate",
						JobImage:   "ghcr.io/acme/pre-gate:1",
						Status:     "Failed",
						ExitCode:   &failCode,
						DurationMs: 1500,
					},
					{
						JobID:         healID,
						JobType:       "heal",
						JobImage:      "ghcr.io/acme/heal:1",
						Status:        "Success",
						ExitCode:      &healCode,
						DurationMs:    1200,
						ActionSummary: "Applied import fix and retried build",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, TextRenderOptions{EnableOSC8: false}); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}
	out := buf.String()

	assertContains(t, out, "✗")
	assertContains(t, out, "pre_gate")
	assertContains(t, out, "└  Exit 137: \x1b[91mcompile failed at step 2\x1b[0m")
	assertContains(t, out, "✓")
	assertContains(t, out, "Heal")
	assertContains(t, out, "└  Exit 0: Applied import fix and retried build")
}

func TestRenderRunReportTextOSC8OnAndOff(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	repoID := domaintypes.NewMigRepoID()
	logURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs"
	patchURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs?download=true&diff_id=abc"
	baseURL, err := url.Parse("https://example.test")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	report := RunReport{
		RunID:   runID,
		MigID:   migID,
		MigName: "links-run",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RepoReport{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/links.git",
				BaseRef:     "main",
				TargetRef:   "ploy/links",
				Status:      "Success",
				Attempt:     1,
				BuildLogURL: logURL,
				PatchURL:    patchURL,
			},
		},
		Runs: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/links.git",
				BaseRef:     "main",
				TargetRef:   "ploy/links",
				Status:      "Success",
				Attempt:     1,
				BuildLogURL: logURL,
				PatchURL:    patchURL,
				Jobs: []RunJobEntry{
					{
						JobID:       domaintypes.NewJobID(),
						JobType:     "mig",
						JobImage:    "ghcr.io/acme/mig:1",
						Status:      "Success",
						DurationMs:  1000,
						BuildLogURL: logURL,
						PatchURL:    patchURL,
					},
				},
			},
		},
	}

	var plain bytes.Buffer
	if err := RenderRunReportText(&plain, report, TextRenderOptions{EnableOSC8: false, AuthToken: "test-token", BaseURL: baseURL}); err != nil {
		t.Fatalf("RenderRunReportText plain error: %v", err)
	}
	plainOut := plain.String()
	assertContains(t, plainOut, "Logs ("+logURL+"?auth_token=test-token)")
	assertContains(t, plainOut, "Download (https://example.test/v1/migs/"+migID.String()+"/specs/latest?auth_token=test-token)")
	assertContains(t, plainOut, "github.com/acme/links (https://github.com/acme/links.git)")
	assertNotContains(t, plainOut, "https://github.com/acme/links.git?auth_token=")
	assertContains(t, plainOut, "Patch (https://example.test/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs?")
	assertContains(t, plainOut, "auth_token=test-token")
	assertContains(t, plainOut, "diff_id=abc")
	assertContains(t, plainOut, "download=true")
	if strings.Contains(plainOut, "\x1b]8;;") {
		t.Fatalf("plain output unexpectedly contains OSC8 sequence: %q", plainOut)
	}

	var linked bytes.Buffer
	if err := RenderRunReportText(&linked, report, TextRenderOptions{EnableOSC8: true, AuthToken: "test-token", BaseURL: baseURL}); err != nil {
		t.Fatalf("RenderRunReportText linked error: %v", err)
	}
	linkedOut := linked.String()
	assertContains(t, linkedOut, "\x1b]8;;"+logURL+"?auth_token=test-token")
	assertContains(t, linkedOut, "\x1b]8;;https://example.test/v1/migs/"+migID.String()+"/specs/latest?auth_token=test-token")
	assertContains(t, linkedOut, "\x1b]8;;https://github.com/acme/links.git\x1b\\github.com/acme/links\x1b]8;;\x1b\\")
	assertNotContains(t, linkedOut, "github.com/acme/links.git?auth_token=")
	assertContains(t, linkedOut, "\x1b]8;;https://example.test/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs?")
	assertContains(t, linkedOut, "auth_token=test-token")
}

func TestRenderRunReportTextArtifactsHiddenForCancelledJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	logURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs"

	report := RunReport{
		RunID:   runID,
		MigID:   domaintypes.NewMigID(),
		MigName: "cancelled-run",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RepoReport{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/cancelled.git",
				BaseRef:     "main",
				TargetRef:   "ploy/cancelled",
				Status:      "Cancelled",
				Attempt:     1,
				BuildLogURL: logURL,
			},
		},
		Runs: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/cancelled.git",
				BaseRef:     "main",
				TargetRef:   "ploy/cancelled",
				Status:      "Cancelled",
				Attempt:     1,
				BuildLogURL: logURL,
				Jobs: []RunJobEntry{
					{
						JobID:       domaintypes.NewJobID(),
						JobType:     "mig",
						JobImage:    "ghcr.io/acme/mig:1",
						Status:      "Cancelled",
						DurationMs:  0,
						BuildLogURL: logURL,
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, TextRenderOptions{EnableOSC8: false}); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}
	out := buf.String()

	assertNotContains(t, out, "Logs (")
}

func TestRenderRunReportTextMigHeaderOnlyIDWhenNameMatches(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   migID,
		MigName: migID.String(),
		SpecID:  domaintypes.NewSpecID(),
		Repos:   []RepoReport{},
	}

	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, TextRenderOptions{}); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}
	out := buf.String()
	assertContains(t, out, "Mig:   "+migID.String()+"\n")
	firstLine := strings.SplitN(out, "\n", 2)[0]
	assertNotContains(t, firstLine, "|")
}

func TestRenderRunReportTextSpinnerFrameAndLiveDuration(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()
	startedAt := time.Date(2026, time.February, 26, 10, 0, 0, 0, time.UTC)

	report := RunReport{
		RunID:   runID,
		MigID:   domaintypes.NewMigID(),
		MigName: "live-run",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RepoReport{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/live.git",
				BaseRef:   "main",
				TargetRef: "ploy/live",
				Status:    "Running",
				Attempt:   1,
			},
		},
		Runs: []RunEntry{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/live.git",
				BaseRef:   "main",
				TargetRef: "ploy/live",
				Status:    "Running",
				Attempt:   1,
				Jobs: []RunJobEntry{
					{
						JobID:      jobID,
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:live",
						Status:     "Running",
						StartedAt:  &startedAt,
						DurationMs: 0,
					},
				},
			},
		},
	}

	now := time.Date(2026, time.February, 26, 10, 0, 5, 0, time.UTC)
	var frame0 bytes.Buffer
	if err := RenderRunReportText(&frame0, report, TextRenderOptions{
		EnableOSC8:    false,
		SpinnerFrame:  0,
		LiveDurations: true,
		Now:           now,
	}); err != nil {
		t.Fatalf("RenderRunReportText frame0 error: %v", err)
	}
	out0 := frame0.String()
	assertContains(t, out0, "⣾")
	assertContains(t, out0, "5.0s")

	var frame1 bytes.Buffer
	if err := RenderRunReportText(&frame1, report, TextRenderOptions{
		EnableOSC8:    false,
		SpinnerFrame:  1,
		LiveDurations: true,
		Now:           now,
	}); err != nil {
		t.Fatalf("RenderRunReportText frame1 error: %v", err)
	}
	out1 := frame1.String()
	assertContains(t, out1, "⣷")
	assertContains(t, out1, "5.0s")
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got: %q", needle, haystack)
	}
}

func assertNotContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected output not to contain %q, got: %q", needle, haystack)
	}
}
