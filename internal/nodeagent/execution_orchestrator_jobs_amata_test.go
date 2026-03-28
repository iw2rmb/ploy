package nodeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestSelectedModAmata(t *testing.T) {
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
				Execution: ModContainerSpec{Amata: amataExec},
			},
			stepIdx:  0,
			wantSpec: "exec-spec",
		},
		{
			name: "multi_step_returns_selected_step_amata",
			typed: RunOptions{
				Steps: []StepMod{
					{ModContainerSpec: ModContainerSpec{}},
					{ModContainerSpec: ModContainerSpec{Amata: amataStep}},
				},
			},
			stepIdx:  1,
			wantSpec: "step-spec",
		},
		{
			name: "multi_step_out_of_range_returns_nil",
			typed: RunOptions{
				Steps: []StepMod{{ModContainerSpec: ModContainerSpec{Amata: amataStep}}},
			},
			stepIdx:  2,
			wantSpec: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := selectedModAmata(tt.typed, tt.stepIdx)
			if tt.wantSpec == "" {
				if got != nil {
					t.Fatalf("selectedModAmata() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("selectedModAmata() = nil, want non-nil")
			}
			if got.Spec != tt.wantSpec {
				t.Fatalf("selectedModAmata().Spec = %q, want %q", got.Spec, tt.wantSpec)
			}
		})
	}
}

func TestWriteAmataSpecInDir(t *testing.T) {
	t.Parallel()

	t.Run("writes_amata_yaml_when_spec_present", func(t *testing.T) {
		t.Parallel()
		inDir := t.TempDir()
		amata := &contracts.AmataRunSpec{
			Spec: "version: amata/v1\nname: test\n",
		}

		if err := writeAmataSpecInDir(inDir, amata); err != nil {
			t.Fatalf("writeAmataSpecInDir() error = %v", err)
		}

		data, err := os.ReadFile(filepath.Join(inDir, "amata.yaml"))
		if err != nil {
			t.Fatalf("read amata.yaml: %v", err)
		}
		want := strings.TrimSpace(amata.Spec)
		if string(data) != want {
			t.Fatalf("amata.yaml content = %q, want %q", string(data), want)
		}
	})

	t.Run("no_op_for_nil_or_empty_spec", func(t *testing.T) {
		t.Parallel()
		tests := []*contracts.AmataRunSpec{
			nil,
			{Spec: ""},
			{Spec: "   "},
		}
		for _, amata := range tests {
			inDir := t.TempDir()
			if err := writeAmataSpecInDir(inDir, amata); err != nil {
				t.Fatalf("writeAmataSpecInDir() error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(inDir, "amata.yaml")); !os.IsNotExist(err) {
				t.Fatalf("expected amata.yaml absent, stat err=%v", err)
			}
		}
	})
}
