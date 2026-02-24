package runs

import (
	"bytes"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRenderRunReportTextIncludesFollowStyleSnapshot(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()

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
				Attempt:     2,
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
				PatchURL:    "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs?download=true&diff_id=abc",
			},
		},
		Runs: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/service.git",
				BaseRef:     "main",
				TargetRef:   "ploy/java17",
				Attempt:     2,
				Status:      "Running",
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
				Jobs: []RunJobEntry{
					{
						JobID:      jobID,
						JobType:    "step",
						JobImage:   "ghcr.io/acme/runner:1",
						Status:     "Running",
						DurationMs: 2450,
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
	assertContains(t, out, "Mig Name: java17-upgrade")
	assertContains(t, out, "Repo: github.com/acme/service main -> ploy/java17")
	assertContains(t, out, "Build Log: build-log (https://example.test/v1/runs/")
	assertContains(t, out, "State")
	assertContains(t, out, "[RUN]")
	assertContains(t, out, "2.5s")
}

func TestRenderRunReportTextStatusScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		report      RunReport
		wantStatus  string
		wantContain string
	}{
		{
			name: "success",
			report: RunReport{
				RunID:   domaintypes.NewRunID(),
				MigID:   domaintypes.NewMigID(),
				MigName: "ok-run",
				SpecID:  domaintypes.NewSpecID(),
				Repos: []RepoReport{
					{
						RepoID:    domaintypes.NewMigRepoID(),
						RepoURL:   "https://github.com/acme/ok.git",
						BaseRef:   "main",
						TargetRef: "ploy/ok",
						Status:    "Success",
						Attempt:   1,
					},
				},
			},
			wantStatus:  "Status: Success",
			wantContain: "Repo: github.com/acme/ok main -> ploy/ok",
		},
		{
			name: "fail",
			report: RunReport{
				RunID:   domaintypes.NewRunID(),
				MigID:   domaintypes.NewMigID(),
				MigName: "fail-run",
				SpecID:  domaintypes.NewSpecID(),
				Repos: []RepoReport{
					{
						RepoID:    domaintypes.NewMigRepoID(),
						RepoURL:   "https://github.com/acme/fail.git",
						BaseRef:   "main",
						TargetRef: "ploy/fail",
						Status:    "Fail",
						Attempt:   1,
						LastError: strPtr("compile\nfailed at step 2"),
					},
				},
			},
			wantStatus:  "Status: Fail",
			wantContain: "Error: compile failed at step 2",
		},
		{
			name: "partial",
			report: RunReport{
				RunID:   domaintypes.NewRunID(),
				MigID:   domaintypes.NewMigID(),
				MigName: "partial-run",
				SpecID:  domaintypes.NewSpecID(),
				Repos: []RepoReport{
					{
						RepoID:    domaintypes.NewMigRepoID(),
						RepoURL:   "https://github.com/acme/a.git",
						BaseRef:   "main",
						TargetRef: "ploy/a",
						Status:    "Success",
						Attempt:   1,
					},
					{
						RepoID:    domaintypes.NewMigRepoID(),
						RepoURL:   "https://github.com/acme/b.git",
						BaseRef:   "main",
						TargetRef: "ploy/b",
						Status:    "Cancelled",
						Attempt:   1,
					},
				},
			},
			wantStatus:  "Status: Partial",
			wantContain: "Repo: github.com/acme/b main -> ploy/b",
		},
		{
			name: "empty repos",
			report: RunReport{
				RunID:   domaintypes.NewRunID(),
				MigID:   domaintypes.NewMigID(),
				MigName: "empty-run",
				SpecID:  domaintypes.NewSpecID(),
				Repos:   []RepoReport{},
				Runs:    []RunEntry{},
			},
			wantStatus:  "Status: Unknown",
			wantContain: "No repos found in this run.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := RenderRunReportText(&buf, tc.report, TextRenderOptions{EnableOSC8: false}); err != nil {
				t.Fatalf("RenderRunReportText error: %v", err)
			}
			out := buf.String()
			assertContains(t, out, tc.wantStatus)
			assertContains(t, out, tc.wantContain)
		})
	}
}

func TestRenderRunReportTextOSC8OnAndOff(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	logURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs"
	patchURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs?download=true&diff_id=abc"

	report := RunReport{
		RunID:   runID,
		MigID:   domaintypes.NewMigID(),
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
	}

	var plain bytes.Buffer
	if err := RenderRunReportText(&plain, report, TextRenderOptions{EnableOSC8: false}); err != nil {
		t.Fatalf("RenderRunReportText plain error: %v", err)
	}
	plainOut := plain.String()
	assertContains(t, plainOut, "build-log ("+logURL+")")
	if strings.Contains(plainOut, "\x1b]8;;") {
		t.Fatalf("plain output unexpectedly contains OSC8 sequence: %q", plainOut)
	}

	var linked bytes.Buffer
	if err := RenderRunReportText(&linked, report, TextRenderOptions{EnableOSC8: true}); err != nil {
		t.Fatalf("RenderRunReportText linked error: %v", err)
	}
	linkedOut := linked.String()
	assertContains(t, linkedOut, "\x1b]8;;"+logURL)
	assertContains(t, linkedOut, "\x1b]8;;"+patchURL)
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got: %q", needle, haystack)
	}
}

func strPtr(v string) *string {
	return &v
}
