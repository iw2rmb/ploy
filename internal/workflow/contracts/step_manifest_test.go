package contracts

import (
	"strings"
	"testing"
)

func TestStepManifestValidate(t *testing.T) {
	valid := StepManifest{
		ID:         "mods-orw-apply",
		Name:       "ORW Apply",
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
				SnapshotCID: "bafybaseline",
			},
			{
				Name:      "overlay",
				MountPath: "/workspace",
				Mode:      StepInputModeReadWrite,
				DiffCID:   "bafyoverlay",
			},
		},
		Shift: &StepShiftSpec{
			Profile: "default",
			Env: map[string]string{
				"SHIFT_TIMEOUT": "5m",
			},
		},
		Retention: StepRetentionSpec{
			RetainContainer: true,
			TTL:             "24h",
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
			},
			wantErr: "inputs[1]",
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
			name: "invalid shift profile",
			mutate: func(m *StepManifest) {
				m.Shift.Profile = ""
				m.Shift.Enabled = true
			},
			wantErr: "shift",
		},
		{
			name: "invalid retention ttl",
			mutate: func(m *StepManifest) {
				m.Retention.TTL = "invalid"
			},
			wantErr: "retention",
		},
		{
			name: "retention ttl required when retaining container",
			mutate: func(m *StepManifest) {
				m.Retention.RetainContainer = true
				m.Retention.TTL = ""
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
	}
	if len(src.Env) > 0 {
		clone.Env = make(map[string]string, len(src.Env))
		for k, v := range src.Env {
			clone.Env[k] = v
		}
	}
	if src.Shift != nil {
		shift := *src.Shift
		if len(src.Shift.Env) > 0 {
			shift.Env = make(map[string]string, len(src.Shift.Env))
			for k, v := range src.Shift.Env {
				shift.Env[k] = v
			}
		}
		clone.Shift = &shift
	}
	return clone
}
