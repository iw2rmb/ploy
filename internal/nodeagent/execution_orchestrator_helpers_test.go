package nodeagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveDockerRegistryAuthConfig(t *testing.T) {
	authFile := filepath.Join(t.TempDir(), "docker-auth.json")
	if err := os.WriteFile(authFile, []byte("  {\"auths\":{\"file.example\":{\"auth\":\"file\"}}}\n"), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	missingFile := filepath.Join(t.TempDir(), "missing.json")

	tests := []struct {
		name       string
		ployAuth   string
		filePath   string
		dockerAuth string
		want       string
		wantErr    string
	}{
		{
			name:       "file_used_when_inline_env_present",
			ployAuth:   `{"auths":{"inline.example":{"auth":"inline"}}}`,
			filePath:   authFile,
			dockerAuth: `{"auths":{"docker.example":{"auth":"docker"}}}`,
			want:       `{"auths":{"file.example":{"auth":"file"}}}`,
		},
		{
			name:       "inline_env_ignored_when_file_empty",
			ployAuth:   `{"auths":{"inline.example":{"auth":"inline"}}}`,
			dockerAuth: `{"auths":{"docker.example":{"auth":"docker"}}}`,
			want:       "",
		},
		{
			name:       "file_used_when_inline_empty",
			filePath:   authFile,
			dockerAuth: `{"auths":{"docker.example":{"auth":"docker"}}}`,
			want:       `{"auths":{"file.example":{"auth":"file"}}}`,
		},
		{
			name:       "docker_auth_ignored",
			dockerAuth: `{"auths":{"docker.example":{"auth":"docker"}}}`,
			want:       "",
		},
		{
			name:     "configured_file_read_error",
			filePath: missingFile,
			wantErr:  "read PLOY_DOCKER_AUTH_CONFIG_FILE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLOY_DOCKER_AUTH_CONFIG", tt.ployAuth)
			t.Setenv("PLOY_DOCKER_AUTH_CONFIG_FILE", tt.filePath)
			t.Setenv("DOCKER_AUTH_CONFIG", tt.dockerAuth)

			got, err := resolveDockerRegistryAuthConfig()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveDockerRegistryAuthConfig() error = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDockerRegistryAuthConfig() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveDockerRegistryAuthConfig() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithTempDir(t *testing.T) {
	tests := []struct {
		name    string
		fnErr   error
		wantErr error
	}{
		{name: "creates_and_cleans_up", fnErr: nil, wantErr: nil},
		{name: "cleans_up_on_error", fnErr: os.ErrInvalid, wantErr: os.ErrInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedDir string
			err := withTempDir("test-*", func(dir string) error {
				capturedDir = dir
				if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
					t.Fatalf("temp directory does not exist: %s", dir)
				}
				if tt.fnErr == nil {
					testFile := filepath.Join(dir, "test.txt")
					if writeErr := os.WriteFile(testFile, []byte("test"), 0o644); writeErr != nil {
						t.Fatalf("failed to create test file: %v", writeErr)
					}
				}
				return tt.fnErr
			})
			if err != tt.wantErr {
				t.Fatalf("withTempDir error = %v, want %v", err, tt.wantErr)
			}
			if _, statErr := os.Stat(capturedDir); !os.IsNotExist(statErr) {
				t.Fatalf("temp directory was not cleaned up: %s", capturedDir)
			}
		})
	}
}

func TestClearManifestHydration(t *testing.T) {
	tests := []struct {
		name   string
		inputs []contracts.StepInput
	}{
		{
			name: "removes_hydration",
			inputs: []contracts.StepInput{
				{Name: "input1", Hydration: &contracts.StepInputHydration{}},
				{Name: "input2", Hydration: &contracts.StepInputHydration{}},
			},
		},
		{
			name:   "empty_inputs_noop",
			inputs: []contracts.StepInput{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{Inputs: tt.inputs}
			clearManifestHydration(&manifest)
			for i, input := range manifest.Inputs {
				if input.Hydration != nil {
					t.Errorf("input[%d].Hydration should be nil", i)
				}
			}
		})
	}
}

func TestDisableManifestGate(t *testing.T) {
	tests := []struct {
		name string
		gate *contracts.StepGateSpec
	}{
		{name: "sets_gate_disabled", gate: &contracts.StepGateSpec{Enabled: true}},
		{name: "nil_gate_sets_disabled", gate: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{Gate: tt.gate}
			disableManifestGate(&manifest)
			if manifest.Gate == nil {
				t.Fatal("Gate should not be nil")
			}
			if manifest.Gate.Enabled {
				t.Error("Gate.Enabled should be false")
			}
		})
	}
}

func TestTempResource_Cleanup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) tempResource
		check func(t *testing.T, tr tempResource)
	}{
		{
			name: "removes directory",
			setup: func(t *testing.T) tempResource {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o644); err != nil {
					t.Fatalf("create test file: %v", err)
				}
				return tempResource{path: dir, cleanup: func() { _ = os.RemoveAll(dir) }}
			},
			check: func(t *testing.T, tr tempResource) {
				tr.cleanup()
				if _, err := os.Stat(tr.path); !os.IsNotExist(err) {
					t.Fatalf("directory was not cleaned up: %s", tr.path)
				}
			},
		},
		{
			name: "noop cleanup is safe",
			setup: func(t *testing.T) tempResource {
				return tempResource{path: "", cleanup: func() {}}
			},
			check: func(t *testing.T, tr tempResource) {
				tr.cleanup() // must not panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tr := tt.setup(t)
			tt.check(t, tr)
		})
	}
}

func TestPrepareStickyWorkspaceWithCleanup_StickyWorkspaceIsNotRemoved(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	req := StartRunRequest{
		RunID:  types.RunID("run_sticky_cleanup"),
		RepoID: types.MigRepoID("repo_sticky_cleanup"),
		JobID:  types.JobID("job_sticky_cleanup"),
	}
	workspace := runRepoWorkspaceDir(req.RunID, req.RepoID)
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sticky .git dir: %v", err)
	}

	rc := &runController{cfg: Config{}}
	result, err := rc.prepareStickyWorkspaceWithCleanup(context.Background(), req, contracts.StepManifest{})
	if err != nil {
		t.Fatalf("prepareStickyWorkspaceWithCleanup() error = %v", err)
	}
	if result.path != workspace {
		t.Fatalf("prepareStickyWorkspaceWithCleanup() path = %q, want %q", result.path, workspace)
	}

	result.cleanup()
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("sticky workspace should not be removed by cleanup, stat err = %v", err)
	}
}
