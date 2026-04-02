package nodeagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestSelectedMigAmata(t *testing.T) {
	t.Parallel()

	amataExec := &contracts.AmataRunSpec{Spec: "exec-spec"}
	amataStep := &contracts.AmataRunSpec{Spec: "step-spec"}

	tests := []struct {
		name     string
		typed    RunOptions
		stepIdx  int
		wantSpec string
	}{
		{
			name: "single_step_returns_execution_amata",
			typed: RunOptions{
				Execution: MigContainerSpec{Amata: amataExec},
			},
			stepIdx:  0,
			wantSpec: "exec-spec",
		},
		{
			name: "multi_step_returns_selected_step_amata",
			typed: RunOptions{
				Steps: []StepMig{
					{MigContainerSpec: MigContainerSpec{}},
					{MigContainerSpec: MigContainerSpec{Amata: amataStep}},
				},
			},
			stepIdx:  1,
			wantSpec: "step-spec",
		},
		{
			name: "multi_step_out_of_range_returns_nil",
			typed: RunOptions{
				Steps: []StepMig{{MigContainerSpec: MigContainerSpec{Amata: amataStep}}},
			},
			stepIdx:  2,
			wantSpec: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := selectedMigAmata(tt.typed, tt.stepIdx)
			if tt.wantSpec == "" {
				if got != nil {
					t.Fatalf("selectedMigAmata() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("selectedMigAmata() = nil, want non-nil")
			}
			if got.Spec != tt.wantSpec {
				t.Fatalf("selectedMigAmata().Spec = %q, want %q", got.Spec, tt.wantSpec)
			}
		})
	}
}

func TestWriteAmataSpecInDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		amata       *contracts.AmataRunSpec
		wantFile    bool
		wantContent string
	}{
		{
			name:        "writes amata yaml when spec present",
			amata:       &contracts.AmataRunSpec{Spec: "version: amata/v1\nname: test\n"},
			wantFile:    true,
			wantContent: "version: amata/v1\nname: test",
		},
		{name: "nil spec noop", amata: nil},
		{name: "empty spec noop", amata: &contracts.AmataRunSpec{Spec: ""}},
		{name: "whitespace spec noop", amata: &contracts.AmataRunSpec{Spec: "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inDir := t.TempDir()
			if err := writeAmataSpecInDir(inDir, tt.amata); err != nil {
				t.Fatalf("writeAmataSpecInDir() error = %v", err)
			}
			path := filepath.Join(inDir, "amata.yaml")
			if !tt.wantFile {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("expected amata.yaml absent, stat err=%v", err)
				}
				return
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read amata.yaml: %v", err)
			}
			if string(data) != tt.wantContent {
				t.Fatalf("amata.yaml content = %q, want %q", string(data), tt.wantContent)
			}
		})
	}
}
