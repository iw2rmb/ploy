package nodeagent

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// testJobImage is a helper to create a JobImage from a string for tests.
func testJobImage(image string) contracts.JobImage {
	return contracts.JobImage{Universal: image}
}

// assertCommandEqual compares two command slices element-by-element with clear diagnostics.
func assertCommandEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Command len: got %d, want %d: %v", len(got), len(want), got)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("Command[%d]: got %q, want %q", i, got[i], v)
		}
	}
}

// assertEnvContains verifies that every key in want is present with the correct value.
func assertEnvContains(t *testing.T, env, want map[string]string) {
	t.Helper()
	for key, wantVal := range want {
		gotVal, ok := env[key]
		if !ok {
			t.Errorf("env missing key %q", key)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("env[%q] = %q, want %q", key, gotVal, wantVal)
		}
	}
}

// assertEnvAbsent verifies that none of the given keys are present.
func assertEnvAbsent(t *testing.T, env map[string]string, keys []string) {
	t.Helper()
	for _, key := range keys {
		if val, ok := env[key]; ok {
			t.Errorf("env[%q] = %q, want key absent", key, val)
		}
	}
}

// TestBuildHealingManifest_RepoMetadataInjection verifies that repo metadata
// env vars (PLOY_REPO_URL, PLOY_BASE_REF, PLOY_TARGET_REF, PLOY_COMMIT_SHA)
// are injected into healing manifests from the StartRunRequest.
func TestBuildHealingManifest_RepoMetadataInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       StartRunRequest
		mig       MigContainerSpec
		wantEnv   map[string]string
		wantAbsnt []string // keys that should NOT be present
	}{
		{
			name: "all repo metadata present",
			req: newStartRunRequest(
				withRunID("test-run-1"), withJobID("test-job-1"),
				withRunRepoURL("https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"),
				withRunBaseRef("main"),
				withRunTargetRef("e2e/fail-missing-symbol"),
				withRunCommitSHA("abc123def456"),
			),
			mig: MigContainerSpec{Image: testJobImage("test/healer:latest")},
			wantEnv: map[string]string{
				"PLOY_REPO_URL":   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				"PLOY_BASE_REF":   "main",
				"PLOY_TARGET_REF": "e2e/fail-missing-symbol",
				"PLOY_COMMIT_SHA": "abc123def456",
			},
		},
		{
			name: "only repo_url and base_ref present",
			req: newStartRunRequest(
				withRunID("test-run-2"), withJobID("test-job-2"),
				withRunRepoURL("https://gitlab.com/test/repo.git"),
				withRunBaseRef("develop"),
				withRunTargetRef(""), withRunCommitSHA(""),
			),
			mig: MigContainerSpec{Image: testJobImage("test/healer:latest")},
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git",
				"PLOY_BASE_REF": "develop",
			},
			wantAbsnt: []string{"PLOY_TARGET_REF", "PLOY_COMMIT_SHA"},
		},
		{
			name: "empty strings are not injected",
			req: newStartRunRequest(
				withRunID("test-run-3"), withJobID("test-job-3"),
				withRunRepoURL("https://gitlab.com/test/repo.git"),
				withRunBaseRef(""),
				withRunTargetRef("   "), // whitespace only
				withRunCommitSHA(""),
			),
			mig: MigContainerSpec{Image: testJobImage("test/healer:latest")},
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git",
			},
			wantAbsnt: []string{"PLOY_BASE_REF", "PLOY_TARGET_REF", "PLOY_COMMIT_SHA"},
		},
		{
			name: "mig env is preserved alongside repo metadata",
			req: newStartRunRequest(
				withRunID("test-run-4"), withJobID("test-job-4"),
				withRunRepoURL("https://gitlab.com/test/repo.git"),
				withRunBaseRef("main"),
				withRunTargetRef(""), withRunCommitSHA(""),
			),
			mig: MigContainerSpec{
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
			req: newStartRunRequest(
				withRunID("test-run-5"), withJobID("test-job-5"),
				withRunRepoURL("https://gitlab.com/test/repo.git"),
				withRunBaseRef("main"),
				withRunTargetRef(""), withRunCommitSHA(""),
			),
			mig: MigContainerSpec{
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
		{
			name: "nil env still injects repo metadata",
			req: newStartRunRequest(
				withRunID("test-run-nil-env"), withJobID("test-job-nil-env"),
				withRunRepoURL("https://gitlab.com/test/repo.git"),
				withRunBaseRef("main"),
				withRunTargetRef(""), withRunCommitSHA(""),
			),
			mig: MigContainerSpec{
				Image: testJobImage("test/healer:latest"),
				Env:   nil, // explicitly nil
			},
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git",
				"PLOY_BASE_REF": "main",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := buildHealingManifest(tc.req, tc.mig, 0, "", contracts.MigStackUnknown)
			if err != nil {
				t.Fatalf("buildHealingManifest() error = %v", err)
			}

			assertEnvContains(t, manifest.Envs, tc.wantEnv)
			assertEnvAbsent(t, manifest.Envs, tc.wantAbsnt)
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

	req := newStartRunRequest(
		withRunID("test-run-mutate"), withJobID("test-job-mutate"),
		withRunRepoURL("https://gitlab.com/test/repo.git"),
		withRunBaseRef("main"),
		withRunTargetRef("feature"), withRunCommitSHA("sha123"),
	)

	mig := MigContainerSpec{
		Image: testJobImage("test/healer:latest"),
		Env:   originalEnv,
	}

	_, err := buildHealingManifest(req, mig, 0, "", contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("buildHealingManifest() error = %v", err)
	}

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

// TestBuildHealingManifest_ValidationErrors verifies error handling for
// invalid healing mig specifications.
func TestBuildHealingManifest_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mig     MigContainerSpec
		wantErr string
	}{
		{
			name:    "empty image",
			mig:     MigContainerSpec{Image: contracts.JobImage{}},
			wantErr: "image required",
		},
		{
			name:    "whitespace only image",
			mig:     MigContainerSpec{Image: contracts.JobImage{Universal: "   "}},
			wantErr: "image required",
		},
	}

	req := newStartRunRequest(
		withRunID("test-run-validation"), withJobID("test-job-validation"),
		withRunRepoURL("https://gitlab.com/test/repo.git"),
		withRunBaseRef("main"),
	)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := buildHealingManifest(req, tc.mig, 0, "", contracts.MigStackUnknown)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestIsAmataHealingImage verifies that the heuristic for detecting Amata-based
// healing images correctly identifies common patterns.
func TestIsAmataHealingImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		image string
		want  bool
	}{
		// Positive cases: images containing "amata".
		{"amata:latest", true},
		{"amata", true},
		{"registry.io/amata:v1", true},
		{"my-amata-healer", true},
		{"AMATA-runner", true}, // case insensitive

		// Negative cases: images without "amata".
		{"codex", false},
		{"registry.io/codex:v1", false},
		{"standard-healer", false},
		{"ubuntu:latest", false},
		{"maven:3.8", false},
		{"mig-fix:latest", false},
		{"amigo-tool", false},
	}

	for _, tc := range tests {
		t.Run(tc.image, func(t *testing.T) {
			t.Parallel()
			got := isAmataHealingImage(tc.image)
			if got != tc.want {
				t.Errorf("isAmataHealingImage(%q) = %v, want %v", tc.image, got, tc.want)
			}
		})
	}
}

// TestBuildHealingManifest_AmataResumeInjection verifies CODEX_RESUME=1 injection
// rules and that user env vars are preserved alongside it.
func TestBuildHealingManifest_AmataResumeInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		mig          MigContainerSpec
		codexSession string
		wantResume   bool              // true if CODEX_RESUME=1 should be set
		wantEnv      map[string]string // additional env assertions (optional)
	}{
		{
			name:         "amata image with session sets CODEX_RESUME=1",
			mig:          MigContainerSpec{Image: testJobImage("amata:latest")},
			codexSession: "session-abc-123",
			wantResume:   true,
		},
		{
			name:         "amata image without session does not set CODEX_RESUME",
			mig:          MigContainerSpec{Image: testJobImage("amata:latest")},
			codexSession: "",
			wantResume:   false,
		},
		{
			name:         "non-amata image with session does not set CODEX_RESUME",
			mig:          MigContainerSpec{Image: testJobImage("standard-healer:v1")},
			codexSession: "session-xyz-456",
			wantResume:   false,
		},
		{
			name:         "non-amata image without session does not set CODEX_RESUME",
			mig:          MigContainerSpec{Image: testJobImage("maven:3.8")},
			codexSession: "",
			wantResume:   false,
		},
		{
			name:         "registry prefixed amata image with session",
			mig:          MigContainerSpec{Image: testJobImage("registry.gitlab.io/ploy/amata:v2")},
			codexSession: "session-def-789",
			wantResume:   true,
		},
		{
			name:         "case insensitive amata detection",
			mig:          MigContainerSpec{Image: testJobImage("my-AMATA-fixer:latest")},
			codexSession: "session-ghi-012",
			wantResume:   true,
		},
		{
			name: "amata resume preserves user env vars",
			mig: MigContainerSpec{
				Image: testJobImage("amata:latest"),
				Env: map[string]string{
					"CUSTOM_VAR": "custom_value",
					"ANOTHER":    "another_value",
				},
			},
			codexSession: "session-id-123",
			wantResume:   true,
			wantEnv: map[string]string{
				"CUSTOM_VAR": "custom_value",
				"ANOTHER":    "another_value",
			},
		},
	}

	req := newStartRunRequest(
		withRunID("test-run-amata-resume"), withJobID("test-job-amata-resume"),
		withRunRepoURL("https://gitlab.com/test/repo.git"),
		withRunBaseRef("main"),
	)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := buildHealingManifest(req, tc.mig, 0, tc.codexSession, contracts.MigStackUnknown)
			if err != nil {
				t.Fatalf("buildHealingManifest() error = %v", err)
			}

			gotResume, hasResume := manifest.Envs["CODEX_RESUME"]
			if tc.wantResume {
				if !hasResume || gotResume != "1" {
					t.Errorf("CODEX_RESUME = %q (present=%v), want '1'", gotResume, hasResume)
				}
			} else {
				if hasResume {
					t.Errorf("CODEX_RESUME = %q, want absent", gotResume)
				}
			}

			assertEnvContains(t, manifest.Envs, tc.wantEnv)
		})
	}
}
