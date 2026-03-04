package runs

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
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
		Repos: []RunEntry{
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
	assertx.Contains(t, out, "   Mig:   "+migID.String()+"   | java17-upgrade")
	assertx.Contains(t, out, "   Spec:  "+specID.String()+" | Download (https://example.test/v1/migs/"+migID.String()+"/specs/latest)")
	assertx.Contains(t, out, "   Repos: 1")
	assertx.Contains(t, out, "\n   Repos: 1\n   Run:   "+runID.String()+"\n\n")
	assertx.Contains(t, out, "   [1/1] github.com/acme/service (https://github.com/acme/service.git) main -> ploy/java17")
	assertx.Contains(t, out, "Artefacts")
	assertx.NotContains(t, out, "State")
	if strings.Count(out, "Logs (https://example.test/v1/runs/") != 1 {
		t.Fatalf("expected exactly one logs link in output, got: %q", out)
	}
	if strings.Count(out, "Patch (https://example.test/v1/runs/") != 1 {
		t.Fatalf("expected exactly one patch link in output, got: %q", out)
	}
	assertx.Contains(t, out, "Logs (https://example.test/v1/runs/")
	assertx.Contains(t, out, " | Patch (https://example.test/v1/runs/")
	assertx.Contains(t, out, "⣾")
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
		Repos: []RunEntry{
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
						Recovery:   &RunJobRecovery{ErrorKind: "infra"},
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

	assertx.Contains(t, out, "\x1b[91m✗\x1b[0m")
	assertx.Contains(t, out, "pre_gate")
	assertx.Contains(t, out, "└  Exit 137: \x1b[91minfra compile failed at step 2\x1b[0m")
	assertx.NotContains(t, out, "<infra>")
	assertx.Contains(t, out, "✓")
	assertx.Contains(t, out, "Heal")
	assertx.Contains(t, out, "└  Exit 0: Applied import fix and retried build")
}

func TestRenderRunReportTextExitOneLinerPrefersBugSummary(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	failID := domaintypes.NewJobID()
	failCode := int32(1)

	report := RunReport{
		RunID:   runID,
		MigID:   domaintypes.NewMigID(),
		MigName: "bug-summary",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/summary.git",
				BaseRef:   "main",
				TargetRef: "ploy/summary",
				Status:    "Fail",
				Attempt:   1,
				Jobs: []RunJobEntry{
					{
						JobID:      failID,
						JobType:    "pre_gate",
						JobImage:   "ghcr.io/acme/pre-gate:1",
						Status:     "Failed",
						ExitCode:   &failCode,
						DurationMs: 750,
						BugSummary: "missing ; in Foo.java",
						Recovery:   &RunJobRecovery{ErrorKind: "code"},
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
	assertx.Contains(t, out, "└  Exit 1: \x1b[91mcode missing ; in Foo.java\x1b[0m")
	assertx.NotContains(t, out, "<code>")
	assertx.Contains(t, out, "0.8s")
}

func TestRenderRunReportTextGateExitOneLinerDefaultsUnknownErrorKind(t *testing.T) {
	t.Parallel()

	failCode := int32(1)
	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "unknown-kind",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    domaintypes.NewMigRepoID(),
				RepoURL:   "https://github.com/acme/unknown.git",
				BaseRef:   "main",
				TargetRef: "ploy/unknown",
				Status:    "Fail",
				Attempt:   1,
				Jobs: []RunJobEntry{
					{
						JobID:      domaintypes.NewJobID(),
						JobType:    "re_gate",
						JobImage:   "ghcr.io/acme/re-gate:1",
						Status:     "Failed",
						ExitCode:   &failCode,
						DurationMs: 1000,
						BugSummary: "re-gate failed",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, TextRenderOptions{EnableOSC8: false}); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}
	assertx.Contains(t, buf.String(), "└  Exit 1: \x1b[91munknown re-gate failed\x1b[0m")
	assertx.NotContains(t, buf.String(), "<unknown>")
}

func TestRenderRunReportTextExitOneLinerWrapsAt100Symbols(t *testing.T) {
	t.Parallel()

	failCode := int32(42)
	longSummary := strings.Repeat("x", 210)
	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "wrapping",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    domaintypes.NewMigRepoID(),
				RepoURL:   "https://github.com/acme/wrap.git",
				BaseRef:   "main",
				TargetRef: "ploy/wrap",
				Status:    "Fail",
				Attempt:   1,
				Jobs: []RunJobEntry{
					{
						JobID:      domaintypes.NewJobID(),
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:1",
						Status:     "Failed",
						ExitCode:   &failCode,
						DurationMs: 900,
						BugSummary: longSummary,
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, TextRenderOptions{EnableOSC8: false}); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}

	prefix := "└  Exit 42: "
	indent := strings.Repeat(" ", len(prefix))
	expected := prefix + "\x1b[91m" + strings.Repeat("x", 100) + "\x1b[0m\n" +
		indent + "\x1b[91m" + strings.Repeat("x", 100) + "\x1b[0m\n" +
		indent + "\x1b[91m" + strings.Repeat("x", 10) + "\x1b[0m"
	assertx.Contains(t, buf.String(), expected)
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
		Repos: []RunEntry{
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
	assertx.Contains(t, plainOut, "Logs ("+logURL+"?auth_token=test-token)")
	assertx.Contains(t, plainOut, "Download (https://example.test/v1/migs/"+migID.String()+"/specs/latest?auth_token=test-token)")
	assertx.Contains(t, plainOut, "github.com/acme/links (https://github.com/acme/links.git)")
	assertx.NotContains(t, plainOut, "https://github.com/acme/links.git?auth_token=")
	assertx.Contains(t, plainOut, "Patch (https://example.test/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs?")
	assertx.Contains(t, plainOut, "auth_token=test-token")
	assertx.Contains(t, plainOut, "diff_id=abc")
	assertx.Contains(t, plainOut, "download=true")
	if strings.Contains(plainOut, "\x1b]8;;") {
		t.Fatalf("plain output unexpectedly contains OSC8 sequence: %q", plainOut)
	}

	var linked bytes.Buffer
	if err := RenderRunReportText(&linked, report, TextRenderOptions{EnableOSC8: true, AuthToken: "test-token", BaseURL: baseURL}); err != nil {
		t.Fatalf("RenderRunReportText linked error: %v", err)
	}
	linkedOut := linked.String()
	assertx.Contains(t, linkedOut, "\x1b]8;;"+logURL+"?auth_token=test-token")
	assertx.Contains(t, linkedOut, "\x1b]8;;https://example.test/v1/migs/"+migID.String()+"/specs/latest?auth_token=test-token")
	assertx.Contains(t, linkedOut, "\x1b]8;;https://github.com/acme/links.git\x1b\\github.com/acme/links\x1b]8;;\x1b\\")
	assertx.NotContains(t, linkedOut, "github.com/acme/links.git?auth_token=")
	assertx.Contains(t, linkedOut, "\x1b]8;;https://example.test/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs?")
	assertx.Contains(t, linkedOut, "auth_token=test-token")
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
		Repos: []RunEntry{
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

	assertx.NotContains(t, out, "Logs (")
}

func TestRenderRunReportTextMigHeaderOnlyIDWhenNameMatches(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   migID,
		MigName: migID.String(),
		SpecID:  domaintypes.NewSpecID(),
		Repos:   []RunEntry{},
	}

	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, TextRenderOptions{}); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}
	out := buf.String()
	assertx.Contains(t, out, "   Mig:   "+migID.String()+"\n")
	firstLine := strings.SplitN(out, "\n", 2)[0]
	assertx.NotContains(t, firstLine, "|")
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
		Repos: []RunEntry{
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
	assertx.Contains(t, out0, "⣾")
	assertx.Contains(t, out0, "5.0s")

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
	assertx.Contains(t, out1, "⣷")
	assertx.Contains(t, out1, "5.0s")
}
