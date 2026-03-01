package nodeagent

import (
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// testJobImage is a helper to create a JobImage from a string for tests.
func testJobImage(image string) contracts.JobImage {
	return contracts.JobImage{Universal: image}
}

// TestBuildHealingManifest_RepoMetadataInjection verifies that repo metadata
// env vars (PLOY_REPO_URL, PLOY_BASE_REF, PLOY_TARGET_REF, PLOY_COMMIT_SHA)
// are injected into healing manifests from the StartRunRequest.
func TestBuildHealingManifest_RepoMetadataInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       StartRunRequest
		mig       ModContainerSpec
		wantEnv   map[string]string
		wantAbsnt []string // keys that should NOT be present
	}{
		{
			name: "all repo metadata present",
			req: StartRunRequest{
				RunID:     types.RunID("test-run-1"),
				JobID:     types.JobID("test-job-1"),
				RepoURL:   types.RepoURL("https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("e2e/fail-missing-symbol"),
				CommitSHA: types.CommitSHA("abc123def456"),
			},
			mig: ModContainerSpec{
				Image: testJobImage("test/healer:latest"),
			},
			wantEnv: map[string]string{
				"PLOY_REPO_URL":   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				"PLOY_BASE_REF":   "main",
				"PLOY_TARGET_REF": "e2e/fail-missing-symbol",
				"PLOY_COMMIT_SHA": "abc123def456",
			},
		},
		{
			name: "only repo_url and base_ref present",
			req: StartRunRequest{
				RunID:   types.RunID("test-run-2"),
				JobID:   types.JobID("test-job-2"),
				RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef: types.GitRef("develop"),
			},
			mig: ModContainerSpec{
				Image: testJobImage("test/healer:latest"),
			},
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git",
				"PLOY_BASE_REF": "develop",
			},
			wantAbsnt: []string{"PLOY_TARGET_REF", "PLOY_COMMIT_SHA"},
		},
		{
			name: "empty strings are not injected",
			req: StartRunRequest{
				RunID:     types.RunID("test-run-3"),
				JobID:     types.JobID("test-job-3"),
				RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef:   types.GitRef(""),
				TargetRef: types.GitRef("   "), // whitespace only
				CommitSHA: types.CommitSHA(""),
			},
			mig: ModContainerSpec{
				Image: testJobImage("test/healer:latest"),
			},
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git",
			},
			wantAbsnt: []string{"PLOY_BASE_REF", "PLOY_TARGET_REF", "PLOY_COMMIT_SHA"},
		},
		{
			name: "mig env is preserved alongside repo metadata",
			req: StartRunRequest{
				RunID:   types.RunID("test-run-4"),
				JobID:   types.JobID("test-job-4"),
				RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef: types.GitRef("main"),
			},
			mig: ModContainerSpec{
				Image: testJobImage("test/healer:latest"),
				Env: map[string]string{
					"CUSTOM_VAR":   "custom_value",
					"ANOTHER_VAR":  "another_value",
					"PLOY_SPECIAL": "user-defined", // user-defined PLOY_ var should be preserved
				},
			},
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git",
				"PLOY_BASE_REF": "main",
				"CUSTOM_VAR":    "custom_value",
				"ANOTHER_VAR":   "another_value",
				"PLOY_SPECIAL":  "user-defined",
			},
		},
		{
			name: "mig env can override repo metadata if specified",
			req: StartRunRequest{
				RunID:   types.RunID("test-run-5"),
				JobID:   types.JobID("test-job-5"),
				RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef: types.GitRef("main"),
			},
			mig: ModContainerSpec{
				Image: testJobImage("test/healer:latest"),
				Env: map[string]string{
					"PLOY_REPO_URL": "https://custom.override/repo.git",
				},
			},
			// User-specified env takes precedence; repo metadata is injected after
			// so it overwrites user values. This is the expected behavior so that
			// healing migs always get the correct Git baseline.
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git", // request value wins
				"PLOY_BASE_REF": "main",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
			manifest, err := buildHealingManifest(tc.req, tc.mig, 0, "", contracts.ModStackUnknown)
			if err != nil {
				t.Fatalf("buildHealingManifest() error = %v", err)
			}

			// Check that expected env vars are present with correct values.
			for key, wantVal := range tc.wantEnv {
				gotVal, ok := manifest.Env[key]
				if !ok {
					t.Errorf("manifest.Env missing key %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("manifest.Env[%q] = %q, want %q", key, gotVal, wantVal)
				}
			}

			// Check that absent keys are not present.
			for _, key := range tc.wantAbsnt {
				if val, ok := manifest.Env[key]; ok {
					t.Errorf("manifest.Env[%q] = %q, want key absent", key, val)
				}
			}
		})
	}
}

// TestBuildHealingManifest_DoesNotMutateInputEnv verifies that the mig.Env
// map passed to buildHealingManifest is not mutated by the function.
func TestBuildHealingManifest_DoesNotMutateInputEnv(t *testing.T) {
	t.Parallel()

	originalEnv := map[string]string{
		"ORIGINAL_KEY": "original_value",
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-run-mutate"),
		JobID:     types.JobID("test-job-mutate"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		CommitSHA: types.CommitSHA("sha123"),
	}

	mig := ModContainerSpec{
		Image: testJobImage("test/healer:latest"),
		Env:   originalEnv,
	}

	// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
	_, err := buildHealingManifest(req, mig, 0, "", contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("buildHealingManifest() error = %v", err)
	}

	// Verify original map was not mutated.
	if len(originalEnv) != 1 {
		t.Errorf("original env map was mutated: len = %d, want 1", len(originalEnv))
	}
	if _, hasPLOY := originalEnv["PLOY_REPO_URL"]; hasPLOY {
		t.Error("original env map was mutated: contains PLOY_REPO_URL")
	}
	if originalEnv["ORIGINAL_KEY"] != "original_value" {
		t.Error("original env map value was mutated")
	}
}

// TestBuildHealingManifest_NilEnvHandledGracefully verifies that a nil
// mig.Env map does not cause panics and repo metadata is still injected.
func TestBuildHealingManifest_NilEnvHandledGracefully(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:   types.RunID("test-run-nil-env"),
		JobID:   types.JobID("test-job-nil-env"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	mig := ModContainerSpec{
		Image: testJobImage("test/healer:latest"),
		Env:   nil, // explicitly nil
	}

	// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
	manifest, err := buildHealingManifest(req, mig, 0, "", contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("buildHealingManifest() error = %v", err)
	}

	// Should still have repo metadata injected.
	if manifest.Env["PLOY_REPO_URL"] != "https://gitlab.com/test/repo.git" {
		t.Errorf("PLOY_REPO_URL = %q, want repo URL", manifest.Env["PLOY_REPO_URL"])
	}
	if manifest.Env["PLOY_BASE_REF"] != "main" {
		t.Errorf("PLOY_BASE_REF = %q, want main", manifest.Env["PLOY_BASE_REF"])
	}
}

// TestBuildHealingManifest_ValidationErrors verifies error handling for
// invalid healing mig specifications.
func TestBuildHealingManifest_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mig     ModContainerSpec
		wantErr string
	}{
		{
			name:    "empty image",
			mig:     ModContainerSpec{Image: contracts.JobImage{}}, // empty JobImage
			wantErr: "image required",
		},
		{
			name:    "whitespace only image",
			mig:     ModContainerSpec{Image: contracts.JobImage{Universal: "   "}},
			wantErr: "image required",
		},
	}

	req := StartRunRequest{
		RunID:   types.RunID("test-run-validation"),
		JobID:   types.JobID("test-job-validation"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
			_, err := buildHealingManifest(req, tc.mig, 0, "", contracts.ModStackUnknown)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestIsCodexHealingImage verifies that the heuristic for detecting Codex-based
// healing images correctly identifies common patterns.
func TestIsCodexHealingImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		image string
		want  bool
	}{
		// Positive cases: images containing "codex".
		{"migs-codex", true},
		{"migs-codex:latest", true},
		{"registry.io/migs-codex:v1", true},
		{"my-codex-healer", true},
		{"codex-fixer", true},
		{"MODS-CODEX", true}, // case insensitive
		{"Codex", true},

		// Negative cases: images without "codex".
		{"standard-healer", false},
		{"ubuntu:latest", false},
		{"maven:3.8", false},
		{"mig-fix:latest", false},
		{"codecov-tool", false}, // "codec" but not "codex"
	}

	for _, tc := range tests {
		t.Run(tc.image, func(t *testing.T) {
			t.Parallel()
			got := isCodexHealingImage(tc.image)
			if got != tc.want {
				t.Errorf("isCodexHealingImage(%q) = %v, want %v", tc.image, got, tc.want)
			}
		})
	}
}

// TestBuildHealingManifest_CodexResumeInjection verifies that CODEX_RESUME=1 is
// injected into the healing manifest environment when:
//  1. A non-empty codexSession is provided, AND
//  2. The healing mig image matches the Codex pattern.
//
// Non-Codex healing migs should never receive CODEX_RESUME.
func TestBuildHealingManifest_CodexResumeInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		mig          ModContainerSpec
		codexSession string
		wantResume   bool // true if CODEX_RESUME=1 should be set
	}{
		{
			name:         "codex image with session sets CODEX_RESUME=1",
			mig:          ModContainerSpec{Image: testJobImage("migs-codex:latest")},
			codexSession: "session-abc-123",
			wantResume:   true,
		},
		{
			name:         "codex image without session does not set CODEX_RESUME",
			mig:          ModContainerSpec{Image: testJobImage("migs-codex:latest")},
			codexSession: "",
			wantResume:   false,
		},
		{
			name:         "non-codex image with session does not set CODEX_RESUME",
			mig:          ModContainerSpec{Image: testJobImage("standard-healer:v1")},
			codexSession: "session-xyz-456",
			wantResume:   false,
		},
		{
			name:         "non-codex image without session does not set CODEX_RESUME",
			mig:          ModContainerSpec{Image: testJobImage("maven:3.8")},
			codexSession: "",
			wantResume:   false,
		},
		{
			name:         "registry prefixed codex image with session",
			mig:          ModContainerSpec{Image: testJobImage("registry.gitlab.io/ploy/migs-codex:v2")},
			codexSession: "session-def-789",
			wantResume:   true,
		},
		{
			name:         "case insensitive codex detection",
			mig:          ModContainerSpec{Image: testJobImage("my-CODEX-fixer:latest")},
			codexSession: "session-ghi-012",
			wantResume:   true,
		},
	}

	req := StartRunRequest{
		RunID:   types.RunID("test-run-codex-resume"),
		JobID:   types.JobID("test-job-codex-resume"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
			manifest, err := buildHealingManifest(req, tc.mig, 0, tc.codexSession, contracts.ModStackUnknown)
			if err != nil {
				t.Fatalf("buildHealingManifest() error = %v", err)
			}

			gotResume, hasResume := manifest.Env["CODEX_RESUME"]
			if tc.wantResume {
				if !hasResume || gotResume != "1" {
					t.Errorf("CODEX_RESUME = %q (present=%v), want '1'", gotResume, hasResume)
				}
			} else {
				if hasResume {
					t.Errorf("CODEX_RESUME = %q, want absent", gotResume)
				}
			}
		})
	}
}

// TestBuildHealingManifest_CodexResumeDoesNotOverrideUserEnv verifies that
// user-specified env vars are preserved when CODEX_RESUME is injected.
func TestBuildHealingManifest_CodexResumeDoesNotOverrideUserEnv(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:   types.RunID("test-run-preserve-env"),
		JobID:   types.JobID("test-job-preserve-env"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	mig := ModContainerSpec{
		Image: testJobImage("migs-codex:latest"),
		Env: map[string]string{
			"CUSTOM_VAR": "custom_value",
			"ANOTHER":    "another_value",
		},
	}

	// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
	manifest, err := buildHealingManifest(req, mig, 0, "session-id-123", contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("buildHealingManifest() error = %v", err)
	}

	// Verify CODEX_RESUME is set.
	if manifest.Env["CODEX_RESUME"] != "1" {
		t.Errorf("CODEX_RESUME = %q, want '1'", manifest.Env["CODEX_RESUME"])
	}

	// Verify user env vars are preserved.
	if manifest.Env["CUSTOM_VAR"] != "custom_value" {
		t.Errorf("CUSTOM_VAR = %q, want 'custom_value'", manifest.Env["CUSTOM_VAR"])
	}
	if manifest.Env["ANOTHER"] != "another_value" {
		t.Errorf("ANOTHER = %q, want 'another_value'", manifest.Env["ANOTHER"])
	}
}

// Note: contains and findSubstring helpers are defined in tls_test.go
