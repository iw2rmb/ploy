package runs

import (
	"net/url"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestOutcomeLinksUseAvailableGateArtifactsAndEncodedToken(t *testing.T) {
	t.Setenv("PLOY_AUTH_TOKEN", "fixture +/&?=")
	base, _ := url.Parse("https://example.test/api?tenant=one")
	pre := migsapi.RunJob{JobID: types.NewJobID(), JobType: types.JobTypePreGate, Status: types.JobStatusFail}
	post := migsapi.RunJob{JobID: types.NewJobID(), JobType: types.JobTypePostGate, Status: types.JobStatusSuccess}
	mig := migsapi.RunJob{JobID: types.NewJobID(), JobType: types.JobTypeMig, Status: types.JobStatusSuccess}
	jobs := []migsapi.RunJob{pre, post, mig}
	artifacts := map[types.JobID]map[string]string{pre.JobID: {"sbom": "pre", "cves": "pre-cves"}, post.JobID: {"sbom": "post", "cves": "post-cves"}}
	for _, tc := range []struct {
		job           migsapi.RunJob
		labels, paths []string
	}{
		{pre, []string{"SBOM", "CVEs"}, []string{"sbom", "cves"}},
		{post, []string{"DIFF", "CVEs"}, []string{"sbom-diff", "cves"}},
		{mig, []string{"Patch"}, []string{}},
	} {
		patch := buildRunPatchURL(base, types.NewRunID(), types.DiffID("11111111-1111-1111-1111-111111111111"), true)
		got := buildJobOutcome(base, tc.job, artifacts, patch, jobs)
		if len(got) != len(tc.labels) {
			t.Fatalf("outcomes=%+v", got)
		}
		for i, item := range got {
			parsed, err := url.Parse(item.URL)
			if err != nil {
				t.Fatal(err)
			}
			if item.Label != tc.labels[i] || parsed.Query().Get("auth_token") != "fixture +/&?=" || parsed.Query().Get("tenant") != "one" {
				t.Fatalf("bad outcome: %+v", item)
			}
			if len(tc.paths) > 0 && parsed.Path != "/api/v1/jobs/"+tc.job.JobID.String()+"/"+tc.paths[i] {
				t.Fatalf("wrong path: %s", parsed.Path)
			}
			if item.Label == "Patch" && (parsed.Query().Get("download") != "true" || parsed.Query().Get("accumulated") != "true" || parsed.Query().Get("diff_id") == "") {
				t.Fatal("lost patch parameters")
			}
		}
	}
	logURL, _ := url.Parse(buildJobLogURL(base, pre.JobID))
	if logURL.Query().Get("auth_token") != "fixture +/&?=" {
		t.Fatal("missing log authentication")
	}
	delete(artifacts, pre.JobID)
	got := buildJobOutcome(base, post, artifacts, "", jobs)
	if len(got) != 1 || got[0].Label != "CVEs" {
		t.Fatalf("missing baseline must leave CVEs available: %+v", got)
	}
}

func TestOutcomeRendersAfterJobIDInPlainAndTerminalOutput(t *testing.T) {
	t.Parallel()
	for _, terminal := range []bool{false, true} {
		job := RunJobEntry{JobID: types.NewJobID(), JobType: types.JobTypePreGate, Status: types.JobStatusFail, JobImage: "image:tag", Outcome: []RunJobOutcome{{Label: "SBOM", URL: "https://example.test/sbom?auth_token=fixture"}, {Label: "CVEs", URL: "https://example.test/cves?auth_token=fixture"}}}
		text := renderText(t, singleJobReport("outcomes", types.RunStatusFail, job), TextRenderOptions{EnableOSC8: terminal})
		plain := ansi.Strip(text)
		if strings.Index(plain, job.JobID.String()) > strings.Index(plain, "SBOM") || !strings.Contains(plain, " | ") || !strings.Contains(plain, "CVEs") || strings.Contains(plain, "image:tag") {
			t.Fatalf("wrong row: %q", plain)
		}
		if !strings.Contains(text, "auth_token=fixture") {
			t.Fatal("missing link authentication")
		}
	}
}
