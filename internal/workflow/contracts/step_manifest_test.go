package contracts

import (
	"strings"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestStepManifestValidate(t *testing.T) {
	valid := StepManifest{
		ID:         types.StepID("mods-sample-apply"),
		Name:       "Sample Apply",
		Image:      "ghcr.io/ploy/mods/openrewrite:latest",
		Command:    []string{"/bin/run"},
		Args:       []string{"--execute"},
		WorkingDir: "/workspace",
		Env: map[string]string{
			"JAVA_TOOL_OPTIONS": "-Xmx2g",
		},
		Inputs: []StepInput{
			{
				Name:        "baseline",
				MountPath:   "/workspace",
				Mode:        StepInputModeReadOnly,
				SnapshotCID: types.CID("bafybaseline"),
				Hydration: &StepInputHydration{
					BaseSnapshot: StepInputArtifactRef{
						CID:    types.CID("bafybaseline"),
						Digest: types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
					},
				},
			},
			{
				Name:      "overlay",
				MountPath: "/workspace",
				Mode:      StepInputModeReadWrite,
				DiffCID:   types.CID("bafyoverlay"),
			},
		},
		Gate: &StepGateSpec{
			Enabled: true,
			Env: map[string]string{
				"GATE_TIMEOUT": "5m",
			},
		},
		Retention: StepRetentionSpec{
			RetainContainer: true,
			TTL:             types.Duration(24 * time.Hour),
		},
	}

	tests := []struct {
		name    string
		mutate  func(m *StepManifest)
		wantErr string
	}{
		{
			name: "valid manifest",
			mutate: func(m *StepManifest) {
				// no-op
			},
		},
		{
			name: "missing id",
			mutate: func(m *StepManifest) {
				m.ID = ""
			},
			wantErr: "id",
		},
		{
			name: "invalid id characters",
			mutate: func(m *StepManifest) {
				m.ID = "Mods Apply"
			},
			wantErr: "id",
		},
		{
			name: "missing image",
			mutate: func(m *StepManifest) {
				m.Image = ""
			},
			wantErr: "image",
		},
		{
			name: "input missing source",
			mutate: func(m *StepManifest) {
				m.Inputs[1].DiffCID = ""
				m.Inputs[1].SnapshotCID = ""
				m.Inputs[1].Hydration = nil
			},
			wantErr: "inputs[1]",
		},
		{
			name: "hydration missing base snapshot",
			mutate: func(m *StepManifest) {
				m.Inputs[0].SnapshotCID = ""
				m.Inputs[0].Hydration = &StepInputHydration{
					Diffs: []StepInputArtifactRef{{CID: types.CID("bafy-diff-1")}},
				}
			},
			wantErr: "inputs[0]",
		},
		{
			name: "hydration base with ordered diffs valid",
			mutate: func(m *StepManifest) {
				m.Inputs[0].Hydration = &StepInputHydration{
					BaseSnapshot: StepInputArtifactRef{
						CID:    types.CID("bafybaseline"),
						Digest: types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
					},
					Diffs: []StepInputArtifactRef{
						{CID: types.CID("bafy-diff-1"), Digest: types.Sha256Digest("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
						{CID: types.CID("bafy-diff-2"), Digest: types.Sha256Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")},
					},
					Repo: &RepoMaterialization{
						URL:       types.RepoURL("https://gitlab.example.com/group/project.git"),
						TargetRef: types.GitRef("refs/heads/main"),
					},
				}
			},
		},
		{
			name: "duplicate input name",
			mutate: func(m *StepManifest) {
				m.Inputs[1].Name = m.Inputs[0].Name
			},
			wantErr: "inputs[1]",
		},
		{
			name: "invalid mount path",
			mutate: func(m *StepManifest) {
				m.Inputs[0].MountPath = "workspace"
			},
			wantErr: "inputs[0]",
		},
		{
			name: "invalid gate env key",
			mutate: func(m *StepManifest) {
				m.Gate.Env = map[string]string{"BAD KEY": "x"}
			},
			wantErr: "gate environment key invalid",
		},
		{
			name: "invalid digest in hydration diff",
			mutate: func(m *StepManifest) {
				m.Inputs[0].Hydration = &StepInputHydration{
					BaseSnapshot: StepInputArtifactRef{
						CID:    types.CID("bafybaseline"),
						Digest: types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
					},
					Diffs: []StepInputArtifactRef{
						{CID: types.CID("bafy-diff-1"), Digest: types.Sha256Digest("bad-digest")},
					},
				}
			},
			wantErr: "digest",
		},
		{
			name: "invalid resources cpu (negative)",
			mutate: func(m *StepManifest) {
				m.Resources.CPU = types.CPUmilli(-1)
			},
			wantErr: "resources",
		},
		{
			name: "retention ttl required when retaining container",
			mutate: func(m *StepManifest) {
				m.Retention.RetainContainer = true
				m.Retention.TTL = 0
			},
			wantErr: "retention",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			manifest := cloneManifest(valid)
			tc.mutate(&manifest)
			err := manifest.Validate()
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

func cloneManifest(src StepManifest) StepManifest {
	clone := src
	if len(src.Command) > 0 {
		clone.Command = append([]string(nil), src.Command...)
	}
	if len(src.Args) > 0 {
		clone.Args = append([]string(nil), src.Args...)
	}
	if len(src.Inputs) > 0 {
		clone.Inputs = make([]StepInput, len(src.Inputs))
		copy(clone.Inputs, src.Inputs)
		for i := range src.Inputs {
			if src.Inputs[i].Hydration != nil {
				h := *src.Inputs[i].Hydration
				if len(src.Inputs[i].Hydration.Diffs) > 0 {
					h.Diffs = append([]StepInputArtifactRef(nil), src.Inputs[i].Hydration.Diffs...)
				}
				clone.Inputs[i].Hydration = &h
			}
		}
	}
	if len(src.Env) > 0 {
		clone.Env = make(map[string]string, len(src.Env))
		for k, v := range src.Env {
			clone.Env[k] = v
		}
	}
	if src.Gate != nil {
		gate := *src.Gate
		if len(src.Gate.Env) > 0 {
			gate.Env = make(map[string]string, len(src.Gate.Env))
			for k, v := range src.Gate.Env {
				gate.Env[k] = v
			}
		}
		if len(src.Gate.DiffPatch) > 0 {
			gate.DiffPatch = append([]byte(nil), src.Gate.DiffPatch...)
		}
		clone.Gate = &gate
	}
	return clone
}
