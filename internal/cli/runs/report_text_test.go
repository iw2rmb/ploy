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

// singleJobReport builds a minimal RunReport with one repo and one job,
// reducing boilerplate in tests that only vary job-level fields.
func singleJobReport(migName string, repoStatus domaintypes.RunRepoStatus, job RunJobEntry) RunReport {
	repoID := domaintypes.NewMigRepoID()
	return RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: migName,
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/" + migName + ".git",
				BaseRef:   "main",
				TargetRef: "ploy/" + migName,
				Status:    repoStatus,
				Attempt:   1,
				Jobs:      []RunJobEntry{job},
			},
		},
	}
}

func renderText(t *testing.T, report RunReport, opts TextRenderOptions) string {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, opts); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}
	return buf.String()
}

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
				RepoID:          repoID,
				RepoURL:         "https://github.com/acme/service.git",
				BaseRef:         "main",
				TargetRef:       "ploy/java17",
				SourceCommitSHA: "0123456789abcdef0123456789abcdef01234567",
				MROnSuccess:     true,
				Attempt:         1,
				Status:          "Running",
				Jobs: []RunJobEntry{
					{
						JobID:      preGateID,
						JobType:    "pre_gate",
						JobImage:   "ghcr.io/acme/pre-gate:1",
						Status:     "Running",
						DurationMs: 2450,
						JobLogURL:  "https://example.test/v1/jobs/" + preGateID.String() + "/logs",
					},
					{
						JobID:      migJobID,
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:1",
						Status:     "Success",
						DurationMs: 3000,
						JobLogURL:  "https://example.test/v1/jobs/" + migJobID.String() + "/logs",
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

	out := renderText(t, report, TextRenderOptions{EnableOSC8: false, BaseURL: baseURL})
	assertx.Contains(t, out, "   Mig:   "+migID.String()+"   | java17-upgrade")
	assertx.Contains(t, out, "   Spec:  "+specID.String()+" (https://example.test/v1/migs/"+migID.String()+"/specs/latest)")
	assertx.Contains(t, out, "   Repos: 1")
	assertx.Contains(t, out, "\n   Repos: 1\n   Run:   "+runID.String()+"\n\n")
	assertx.Contains(t, out, "   ["+repoID.String()+"] github.com/acme/service (https://github.com/acme/service.git) @ "+boldBranchName("main")+" (01234567) -> "+boldBranchName("ploy/java17"))
	assertx.NotContains(t, out, "Artefacts")
	assertx.NotContains(t, out, "State")
	assertx.NotContains(t, out, "Logs (https://example.test/v1/runs/")
	if strings.Count(out, "Patch (https://example.test/v1/runs/") != 1 {
		t.Fatalf("expected exactly one patch link in output, got: %q", out)
	}
	assertx.Contains(t, out, "Patch (https://example.test/v1/runs/")
	assertx.Contains(t, out, migJobID.String()+" (https://example.test/v1/jobs/"+migJobID.String()+"/logs)")
	assertx.Contains(t, out, "⣾")
}

func TestRenderRunReportTextHidesTargetBranchWhenMRFlagsDisabled(t *testing.T) {
	t.Parallel()

	repoID := domaintypes.NewMigRepoID()
	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "no-mr-target",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:          repoID,
				RepoURL:         "https://github.com/acme/no-mr.git",
				BaseRef:         "main",
				TargetRef:       "feature/no-mr",
				SourceCommitSHA: "fedcba9876543210fedcba9876543210fedcba98",
				Status:          "Queued",
				Attempt:         1,
			},
		},
	}

	out := renderText(t, report, TextRenderOptions{EnableOSC8: false})
	assertx.Contains(t, out, "   ["+repoID.String()+"] github.com/acme/no-mr (https://github.com/acme/no-mr.git) @ "+boldBranchName("main")+" (fedcba98)")
	assertx.NotContains(t, out, "feature/no-mr")
	assertx.NotContains(t, out, " -> ")
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
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/heal.git",
				BaseRef:   "main",
				TargetRef: "ploy/heal",
				Attempt:   1,
				Status:    "Fail",
				LastError: &errText,
				Jobs: []RunJobEntry{
					{
						JobID:      preGateID,
						JobType:    "pre_gate",
						JobImage:   "ghcr.io/acme/pre-gate:1",
						Status:     "Failed",
						ExitCode:   &failCode,
						DurationMs: 1500,
						Recovery:   &RunJobRecovery{LoopKind: "healing"},
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

	out := renderText(t, report, TextRenderOptions{EnableOSC8: false})
	assertx.Contains(t, out, ColoredStatusGlyph("failed", 0))
	assertx.Contains(t, out, "pre_gate")
	assertx.Contains(t, out, "└  Exit 137: "+colorizeErrorText("Error"))
	assertx.NotContains(t, out, "infra compile failed at step 2")
	assertx.NotContains(t, out, "<infra>")
	assertx.Contains(t, out, "✓")
	assertx.Contains(t, out, "Heal")
	assertx.Contains(t, out, "└  Exit 0: Applied import fix and retried build")
}

func TestRenderRunReportTextExitOneLinerVariants(t *testing.T) {
	t.Parallel()

	failCode := int32(1)
	failCode42 := int32(42)
	longSummary := strings.Repeat("x", 210)

	prefix42 := "└  Exit 42: "
	indent42 := strings.Repeat(" ", len(prefix42))
	wrappedExpected := prefix42 + colorizeErrorText(strings.Repeat("x", 100)) + "\n" +
		indent42 + colorizeErrorText(strings.Repeat("x", 100)) + "\n" +
		indent42 + colorizeErrorText(strings.Repeat("x", 10))

	tests := []struct {
		name       string
		job        RunJobEntry
		contains   []string
		notContain []string
	}{
		{
			name: "prefers bug summary over error",
			job: RunJobEntry{
				JobID:      domaintypes.NewJobID(),
				JobType:    "pre_gate",
				JobImage:   "ghcr.io/acme/pre-gate:1",
				Status:     "Failed",
				ExitCode:   &failCode,
				DurationMs: 750,
				BugSummary: "missing ; in Foo.java",
				Recovery:   &RunJobRecovery{LoopKind: "healing"},
			},
			contains:   []string{"└  Exit 1: " + colorizeErrorText("Error"), "0.8s"},
			notContain: []string{"<code>"},
		},
		{
			name: "omits unknown prefix when recovery kind is absent",
			job: RunJobEntry{
				JobID:      domaintypes.NewJobID(),
				JobType:    "re_gate",
				JobImage:   "ghcr.io/acme/re-gate:1",
				Status:     "Failed",
				ExitCode:   &failCode,
				DurationMs: 1000,
				BugSummary: "re-gate failed",
			},
			contains:   []string{"└  Exit 1: " + colorizeErrorText("Error")},
			notContain: []string{"<unknown>", "unknown re-gate failed", "re-gate failed"},
		},
		{
			name: "wraps at 100 symbols",
			job: RunJobEntry{
				JobID:      domaintypes.NewJobID(),
				JobType:    "mig",
				JobImage:   "ghcr.io/acme/mig:1",
				Status:     "Failed",
				ExitCode:   &failCode42,
				DurationMs: 900,
				BugSummary: longSummary,
			},
			contains: []string{wrappedExpected},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			report := singleJobReport("exit-variant", "Fail", tc.job)
			out := renderText(t, report, TextRenderOptions{EnableOSC8: false})
			for _, needle := range tc.contains {
				assertx.Contains(t, out, needle)
			}
			for _, needle := range tc.notContain {
				assertx.NotContains(t, out, needle)
			}
		})
	}
}

func TestRenderRunReportTextOSC8OnAndOff(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()
	jobLogURL := "https://example.test/v1/jobs/" + jobID.String() + "/logs"
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
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/links.git",
				BaseRef:   "main",
				TargetRef: "ploy/links",
				Status:    "Success",
				Attempt:   1,
				PatchURL:  patchURL,
				Jobs: []RunJobEntry{
					{
						JobID:      jobID,
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:1",
						Status:     "Success",
						DurationMs: 1000,
						JobLogURL:  jobLogURL,
						PatchURL:   patchURL,
					},
				},
			},
		},
	}

	plainOut := renderText(t, report, TextRenderOptions{EnableOSC8: false, AuthToken: "test-token", BaseURL: baseURL})
	assertx.NotContains(t, plainOut, "Logs (")
	assertx.Contains(t, plainOut, jobID.String()+" ("+jobLogURL+"?auth_token=test-token)")
	assertx.Contains(t, plainOut, report.SpecID.String()+" (https://example.test/v1/migs/"+migID.String()+"/specs/latest?auth_token=test-token)")
	assertx.Contains(t, plainOut, "github.com/acme/links (https://github.com/acme/links.git)")
	assertx.NotContains(t, plainOut, "https://github.com/acme/links.git?auth_token=")
	assertx.Contains(t, plainOut, "Patch (https://example.test/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs?")
	assertx.Contains(t, plainOut, "auth_token=test-token")
	assertx.Contains(t, plainOut, "diff_id=abc")
	assertx.Contains(t, plainOut, "download=true")
	if strings.Contains(plainOut, "\x1b]8;;") {
		t.Fatalf("plain output unexpectedly contains OSC8 sequence: %q", plainOut)
	}

	linkedOut := renderText(t, report, TextRenderOptions{EnableOSC8: true, AuthToken: "test-token", BaseURL: baseURL})
	assertx.Contains(t, linkedOut, "\x1b]8;;"+jobLogURL+"?auth_token=test-token\x1b\\"+jobID.String()+"\x1b]8;;\x1b\\")
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
	cancelledJobID := domaintypes.NewJobID()

	report := RunReport{
		RunID:   runID,
		MigID:   domaintypes.NewMigID(),
		MigName: "cancelled-run",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/cancelled.git",
				BaseRef:   "main",
				TargetRef: "ploy/cancelled",
				Status:    "Cancelled",
				Attempt:   1,
				Jobs: []RunJobEntry{
					{
						JobID:      cancelledJobID,
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:1",
						Status:     "Cancelled",
						DurationMs: 0,
						JobLogURL:  "https://example.test/v1/jobs/" + cancelledJobID.String() + "/logs",
					},
				},
			},
		},
	}

	out := renderText(t, report, TextRenderOptions{EnableOSC8: false})
	assertx.NotContains(t, out, "Patch (")
}

func TestRenderRunReportTextRunningJobIOPreviewCollapsed(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := RunJobEntry{
		JobID:      jobID,
		JobType:    "mig",
		JobImage:   "ghcr.io/acme/mig:1",
		Status:     domaintypes.JobStatusRunning,
		DurationMs: 1000,
	}
	report := singleJobReport("io-collapsed", "Running", job)

	longStdout := strings.Repeat("o", 95)
	out := renderText(t, report, TextRenderOptions{
		EnableOSC8: false,
		JobIOPreviews: map[domaintypes.JobID]RunJobIOPreview{
			jobID: {
				Stdout: []string{"first", longStdout},
				Stderr: []string{"warning line"},
			},
		},
	})

	assertx.Contains(t, out, "STD[O]UT")
	assertx.Contains(t, out, "STD[E]RR")
	assertx.Contains(t, out, strings.Repeat("o", 77)+"...")
	assertx.NotContains(t, out, "\n     first\n")
	assertx.NotContains(t, out, "\n     warning line\n")
}

func TestRenderRunReportTextRunningJobIOPreviewExpanded(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := RunJobEntry{
		JobID:      jobID,
		JobType:    "mig",
		JobImage:   "ghcr.io/acme/mig:1",
		Status:     domaintypes.JobStatusRunning,
		DurationMs: 1000,
	}
	report := singleJobReport("io-expanded", "Running", job)

	wrapCandidate := strings.Repeat("z", 90)
	out := renderText(t, report, TextRenderOptions{
		EnableOSC8:   false,
		ExpandStdout: true,
		ExpandStderr: true,
		JobIOPreviews: map[domaintypes.JobID]RunJobIOPreview{
			jobID: {
				Stdout: []string{"one", "two", wrapCandidate, "tail"},
				Stderr: []string{"err-one", "err-two"},
			},
		},
	})

	assertx.Contains(t, out, "\n     two\n")
	assertx.Contains(t, out, "\n     "+strings.Repeat("z", 80)+"\n")
	assertx.Contains(t, out, "\n     "+strings.Repeat("z", 10)+"\n")
	assertx.Contains(t, out, "\n     tail\n")
	assertx.NotContains(t, out, "STD[O]UT tail")
	assertx.NotContains(t, out, "STD[E]RR err-two")
	assertx.Contains(t, out, colorizeErrorText("err-one"))
	assertx.Contains(t, out, colorizeErrorText("err-two"))
}

func TestRenderRunReportTextFailedJobIOPreviewAlwaysExpanded(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := RunJobEntry{
		JobID:      jobID,
		JobType:    "post_gate",
		JobImage:   "ghcr.io/acme/post-gate:1",
		Status:     domaintypes.JobStatusFail,
		DurationMs: 2200,
	}
	report := singleJobReport("io-failed", "Fail", job)

	out := renderText(t, report, TextRenderOptions{
		EnableOSC8:   false,
		ExpandStdout: false,
		ExpandStderr: false,
		JobIOPreviews: map[domaintypes.JobID]RunJobIOPreview{
			jobID: {
				Stdout: []string{"s1", "s2", "s3"},
				Stderr: []string{"e1"},
			},
		},
	})

	assertx.Contains(t, out, "\n     s1\n")
	assertx.Contains(t, out, "\n     s2\n")
	assertx.Contains(t, out, "\n     s3\n")
	assertx.Contains(t, out, colorizeErrorText("e1"))
}

func TestRenderRunReportTextSucceededJobDoesNotRenderIOPreview(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	job := RunJobEntry{
		JobID:      jobID,
		JobType:    "mig",
		JobImage:   "ghcr.io/acme/mig:1",
		Status:     domaintypes.JobStatusSuccess,
		DurationMs: 1200,
	}
	report := singleJobReport("io-hidden", "Success", job)

	out := renderText(t, report, TextRenderOptions{
		EnableOSC8: false,
		JobIOPreviews: map[domaintypes.JobID]RunJobIOPreview{
			jobID: {
				Stdout: []string{"should not render"},
				Stderr: []string{"should not render"},
			},
		},
		ExpandStdout: true,
		ExpandStderr: true,
	})

	assertx.NotContains(t, out, "STD[O]UT")
	assertx.NotContains(t, out, "STD[E]RR")
	assertx.NotContains(t, out, "should not render")
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

	out := renderText(t, report, TextRenderOptions{})
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

	tests := []struct {
		name  string
		frame int
		glyph string
	}{
		{"frame 0", 0, "⣾"},
		{"frame 1", 1, "⣷"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := renderText(t, report, TextRenderOptions{
				EnableOSC8:    false,
				SpinnerFrame:  tc.frame,
				LiveDurations: true,
				Now:           now,
			})
			assertx.Contains(t, out, tc.glyph)
			assertx.Contains(t, out, "5.0s")
		})
	}
}
