package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestBuildHealingManifest_RepoMetadataInjection verifies that repo metadata
// env vars (PLOY_REPO_URL, PLOY_BASE_REF, PLOY_TARGET_REF, PLOY_COMMIT_SHA)
// are injected into healing manifests from the StartRunRequest.
func TestBuildHealingManifest_RepoMetadataInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       StartRunRequest
		mod       HealingMod
		wantEnv   map[string]string
		wantAbsnt []string // keys that should NOT be present
	}{
		{
			name: "all repo metadata present",
			req: StartRunRequest{
				RunID:     types.RunID("test-run-1"),
				RepoURL:   types.RepoURL("https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("e2e/fail-missing-symbol"),
				CommitSHA: types.CommitSHA("abc123def456"),
			},
			mod: HealingMod{
				Image: "test/healer:latest",
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
				RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef: types.GitRef("develop"),
			},
			mod: HealingMod{
				Image: "test/healer:latest",
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
				RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef:   types.GitRef(""),
				TargetRef: types.GitRef("   "), // whitespace only
				CommitSHA: types.CommitSHA(""),
			},
			mod: HealingMod{
				Image: "test/healer:latest",
			},
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git",
			},
			wantAbsnt: []string{"PLOY_BASE_REF", "PLOY_TARGET_REF", "PLOY_COMMIT_SHA"},
		},
		{
			name: "mod env is preserved alongside repo metadata",
			req: StartRunRequest{
				RunID:   types.RunID("test-run-4"),
				RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef: types.GitRef("main"),
			},
			mod: HealingMod{
				Image: "test/healer:latest",
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
			name: "mod env can override repo metadata if specified",
			req: StartRunRequest{
				RunID:   types.RunID("test-run-5"),
				RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
				BaseRef: types.GitRef("main"),
			},
			mod: HealingMod{
				Image: "test/healer:latest",
				Env: map[string]string{
					"PLOY_REPO_URL": "https://custom.override/repo.git",
				},
			},
			// User-specified env takes precedence; repo metadata is injected after
			// so it overwrites user values. This is the expected behavior so that
			// healing mods always get the correct Git baseline.
			wantEnv: map[string]string{
				"PLOY_REPO_URL": "https://gitlab.com/test/repo.git", // request value wins
				"PLOY_BASE_REF": "main",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := buildHealingManifest(tc.req, tc.mod, 0)
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

// TestBuildHealingManifest_DoesNotMutateInputEnv verifies that the mod.Env
// map passed to buildHealingManifest is not mutated by the function.
func TestBuildHealingManifest_DoesNotMutateInputEnv(t *testing.T) {
	t.Parallel()

	originalEnv := map[string]string{
		"ORIGINAL_KEY": "original_value",
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-run-mutate"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		CommitSHA: types.CommitSHA("sha123"),
	}

	mod := HealingMod{
		Image: "test/healer:latest",
		Env:   originalEnv,
	}

	_, err := buildHealingManifest(req, mod, 0)
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
// mod.Env map does not cause panics and repo metadata is still injected.
func TestBuildHealingManifest_NilEnvHandledGracefully(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:   types.RunID("test-run-nil-env"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	mod := HealingMod{
		Image: "test/healer:latest",
		Env:   nil, // explicitly nil
	}

	manifest, err := buildHealingManifest(req, mod, 0)
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
// invalid healing mod specifications.
func TestBuildHealingManifest_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mod     HealingMod
		wantErr string
	}{
		{
			name:    "empty image",
			mod:     HealingMod{Image: ""},
			wantErr: "image required",
		},
		{
			name:    "whitespace only image",
			mod:     HealingMod{Image: "   "},
			wantErr: "image required",
		},
	}

	req := StartRunRequest{
		RunID:   types.RunID("test-run-validation"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := buildHealingManifest(req, tc.mod, 0)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// Note: contains and findSubstring helpers are defined in tls_test.go
