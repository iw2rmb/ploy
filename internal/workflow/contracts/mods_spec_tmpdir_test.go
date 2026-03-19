package contracts

import (
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestModsSpecValidate_TmpDirStep(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "valid single tmp file",
			input: `{
				"steps": [{"image": "img:latest", "tmp_dir": [{"name": "config.json", "content": "e2Zve30="}]}]
			}`,
		},
		{
			name: "valid multiple tmp files",
			input: `{
				"steps": [{"image": "img:latest", "tmp_dir": [
					{"name": "a.txt", "content": "aGVsbG8="},
					{"name": "b.txt", "content": "d29ybGQ="}
				]}]
			}`,
		},
		{
			name: "empty name rejected",
			input: `{
				"steps": [{"image": "img:latest", "tmp_dir": [{"name": "", "content": "aGVsbG8="}]}]
			}`,
			wantErr: "steps[0].tmp_dir[0].name: required",
		},
		{
			name: "empty content rejected",
			input: `{
				"steps": [{"image": "img:latest", "tmp_dir": [{"name": "file.txt", "content": ""}]}]
			}`,
			wantErr: "steps[0].tmp_dir[0].content: required",
		},
		{
			name: "duplicate names rejected",
			input: `{
				"steps": [{"image": "img:latest", "tmp_dir": [
					{"name": "config.json", "content": "aGVsbG8="},
					{"name": "config.json", "content": "d29ybGQ="}
				]}]
			}`,
			wantErr: `steps[0].tmp_dir[1].name: duplicate "config.json"`,
		},
		{
			name: "second step duplicate names rejected",
			input: `{
				"steps": [
					{"image": "img:latest"},
					{"image": "img2:latest", "tmp_dir": [
						{"name": "x", "content": "aGVsbG8="},
						{"name": "x", "content": "d29ybGQ="}
					]}
				]
			}`,
			wantErr: `steps[1].tmp_dir[1].name: duplicate "x"`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModsSpecJSON([]byte(tc.input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestModsSpecValidate_TmpDirHealing(t *testing.T) {
	base := `{
		"steps": [{"image": "img:latest"}],
		"build_gate": {
			"router": {"image": "router:latest"},
			"healing": {"by_error_kind": {"infra": {"retries": 1, "image": "healer:latest", "tmp_dir": %s}}}
		}
	}`

	tests := []struct {
		name    string
		tmpDir  string
		wantErr string
	}{
		{
			name:   "valid tmp file in healing action",
			tmpDir: `[{"name": "patch.diff", "content": "aGVsbG8="}]`,
		},
		{
			name:    "empty name in healing action rejected",
			tmpDir:  `[{"name": "", "content": "aGVsbG8="}]`,
			wantErr: "build_gate.healing.by_error_kind.infra.tmp_dir[0].name: required",
		},
		{
			name:    "empty content in healing action rejected",
			tmpDir:  `[{"name": "f.txt", "content": ""}]`,
			wantErr: "build_gate.healing.by_error_kind.infra.tmp_dir[0].content: required",
		},
		{
			name:    "duplicate names in healing action rejected",
			tmpDir:  `[{"name": "x", "content": "aGVsbG8="}, {"name": "x", "content": "d29ybGQ="}]`,
			wantErr: `build_gate.healing.by_error_kind.infra.tmp_dir[1].name: duplicate "x"`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := strings.ReplaceAll(base, "%s", tc.tmpDir)
			_, err := ParseModsSpecJSON([]byte(input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestModsSpecValidate_TmpDirRouter(t *testing.T) {
	base := `{
		"steps": [{"image": "img:latest"}],
		"build_gate": {"router": {"image": "router:latest", "tmp_dir": %s}}
	}`

	tests := []struct {
		name    string
		tmpDir  string
		wantErr string
	}{
		{
			name:   "valid tmp file in router",
			tmpDir: `[{"name": "rules.json", "content": "e30="}]`,
		},
		{
			name:    "empty name in router rejected",
			tmpDir:  `[{"name": "", "content": "aGVsbG8="}]`,
			wantErr: "build_gate.router.tmp_dir[0].name: required",
		},
		{
			name:    "duplicate names in router rejected",
			tmpDir:  `[{"name": "cfg", "content": "aGVsbG8="}, {"name": "cfg", "content": "d29ybGQ="}]`,
			wantErr: `build_gate.router.tmp_dir[1].name: duplicate "cfg"`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := strings.ReplaceAll(base, "%s", tc.tmpDir)
			_, err := ParseModsSpecJSON([]byte(input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestStepManifestValidate_TmpDir(t *testing.T) {
	base := StepManifest{
		ID:         types.StepID("migs-sample-apply"),
		Name:       "Sample Apply",
		Image:      "ghcr.io/ploy/migs/runner:latest",
		WorkingDir: "/workspace",
		Inputs: []StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      StepInputModeReadWrite,
				Hydration: &StepInputHydration{
					Repo: &RepoMaterialization{
						URL:       types.RepoURL("https://gitlab.example.com/group/project.git"),
						TargetRef: types.GitRef("refs/heads/main"),
					},
				},
			},
		},
		Gate: &StepGateSpec{Enabled: false},
	}

	tests := []struct {
		name    string
		tmpDir  []TmpFilePayload
		wantErr string
	}{
		{
			name:   "nil tmp_dir valid",
			tmpDir: nil,
		},
		{
			name: "valid single entry",
			tmpDir: []TmpFilePayload{
				{Name: "config.json", Content: []byte(`{"key":"val"}`)},
			},
		},
		{
			name: "valid multiple entries",
			tmpDir: []TmpFilePayload{
				{Name: "a.txt", Content: []byte("hello")},
				{Name: "b.txt", Content: []byte("world")},
			},
		},
		{
			name: "empty name rejected",
			tmpDir: []TmpFilePayload{
				{Name: "", Content: []byte("data")},
			},
			wantErr: "tmp_dir[0].name: required",
		},
		{
			name: "empty content rejected",
			tmpDir: []TmpFilePayload{
				{Name: "file.txt", Content: nil},
			},
			wantErr: "tmp_dir[0].content: required",
		},
		{
			name: "duplicate names rejected",
			tmpDir: []TmpFilePayload{
				{Name: "same.txt", Content: []byte("a")},
				{Name: "same.txt", Content: []byte("b")},
			},
			wantErr: `tmp_dir[1].name: duplicate "same.txt"`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := cloneManifest(base)
			m.TmpDir = tc.tmpDir
			err := m.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
