package contracts

import (
	"testing"
)

func TestBuildGateProfileOverrideToSpecMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		override *BuildGateProfileOverride
		wantNil  bool
		check    func(t *testing.T, m map[string]any)
	}{
		{
			name:    "nil override returns nil",
			wantNil: true,
		},
		{
			name: "shell command",
			override: &BuildGateProfileOverride{
				Command: CommandSpec{Shell: "go test ./..."},
			},
			check: func(t *testing.T, m map[string]any) {
				t.Helper()
				if got, ok := m["command"].(string); !ok || got != "go test ./..." {
					t.Errorf("command: got %v, want string %q", m["command"], "go test ./...")
				}
				if _, ok := m["env"]; ok {
					t.Error("env key should be absent when env is nil")
				}
				if _, ok := m["stack"]; ok {
					t.Error("stack key should be absent when stack is nil")
				}
				if _, ok := m["target"]; ok {
					t.Error("target key should be absent when target is empty")
				}
			},
		},
		{
			name: "exec array command",
			override: &BuildGateProfileOverride{
				Command: CommandSpec{Exec: []string{"go", "test", "./..."}},
			},
			check: func(t *testing.T, m map[string]any) {
				t.Helper()
				exec, ok := m["command"].([]any)
				if !ok {
					t.Fatalf("command: got %T, want []any", m["command"])
				}
				want := []string{"go", "test", "./..."}
				if len(exec) != len(want) {
					t.Fatalf("command length: got %d, want %d", len(exec), len(want))
				}
				for i, v := range want {
					if s, ok := exec[i].(string); !ok || s != v {
						t.Errorf("command[%d]: got %v, want %q", i, exec[i], v)
					}
				}
			},
		},
		{
			name: "env populated",
			override: &BuildGateProfileOverride{
				Command: CommandSpec{Shell: "make test"},
				Env:     map[string]string{"FOO": "bar"},
			},
			check: func(t *testing.T, m map[string]any) {
				t.Helper()
				env, ok := m["env"].(map[string]any)
				if !ok {
					t.Fatalf("env: got %T, want map[string]any", m["env"])
				}
				if v, ok := env["FOO"].(string); !ok || v != "bar" {
					t.Errorf("env[FOO]: got %v, want %q", env["FOO"], "bar")
				}
			},
		},
		{
			name: "nil env omits env key",
			override: &BuildGateProfileOverride{
				Command: CommandSpec{Shell: "make test"},
			},
			check: func(t *testing.T, m map[string]any) {
				t.Helper()
				if _, ok := m["env"]; ok {
					t.Error("env key should be absent when env is nil/empty")
				}
			},
		},
		{
			name: "stack without release",
			override: &BuildGateProfileOverride{
				Command: CommandSpec{Shell: "mvn test"},
				Stack:   &GateProfileStack{Language: "java", Tool: "maven"},
			},
			check: func(t *testing.T, m map[string]any) {
				t.Helper()
				stack, ok := m["stack"].(map[string]any)
				if !ok {
					t.Fatalf("stack: got %T, want map[string]any", m["stack"])
				}
				if v, _ := stack["language"].(string); v != "java" {
					t.Errorf("stack.language: got %q, want java", v)
				}
				if v, _ := stack["tool"].(string); v != "maven" {
					t.Errorf("stack.tool: got %q, want maven", v)
				}
				if _, ok := stack["release"]; ok {
					t.Error("release key should be absent when release is empty")
				}
			},
		},
		{
			name: "stack with release",
			override: &BuildGateProfileOverride{
				Command: CommandSpec{Shell: "mvn test"},
				Stack:   &GateProfileStack{Language: "java", Tool: "maven", Release: "17"},
			},
			check: func(t *testing.T, m map[string]any) {
				t.Helper()
				stack, ok := m["stack"].(map[string]any)
				if !ok {
					t.Fatalf("stack: got %T, want map[string]any", m["stack"])
				}
				if v, _ := stack["release"].(string); v != "17" {
					t.Errorf("stack.release: got %q, want 17", v)
				}
			},
		},
		{
			name: "target populated",
			override: &BuildGateProfileOverride{
				Command: CommandSpec{Shell: "go test ./..."},
				Target:  GateProfileTargetUnit,
			},
			check: func(t *testing.T, m map[string]any) {
				t.Helper()
				if v, _ := m["target"].(string); v != GateProfileTargetUnit {
					t.Errorf("target: got %q, want %q", v, GateProfileTargetUnit)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := BuildGateProfileOverrideToSpecMap(tc.override)
			if tc.wantNil {
				if m != nil {
					t.Fatalf("expected nil, got %v", m)
				}
				return
			}
			if m == nil {
				t.Fatal("expected non-nil map")
			}
			tc.check(t, m)
		})
	}
}

func TestApplyBuildGatePhaseToGateSpec(t *testing.T) {
	t.Parallel()

	t.Run("nil phase is no-op", func(t *testing.T) {
		t.Parallel()
		spec := &StepGateSpec{Target: "build", Always: true}
		ApplyBuildGatePhaseToGateSpec(spec, nil)
		if spec.Target != "build" || !spec.Always {
			t.Error("nil phase should not modify spec")
		}
	})

	t.Run("nil spec is no-op", func(t *testing.T) {
		t.Parallel()
		phase := &BuildGatePhaseConfig{Target: GateProfileTargetUnit}
		ApplyBuildGatePhaseToGateSpec(nil, phase) // must not panic
	})

	t.Run("propagates target and always", func(t *testing.T) {
		t.Parallel()
		spec := &StepGateSpec{}
		phase := &BuildGatePhaseConfig{Target: GateProfileTargetUnit, Always: true}
		ApplyBuildGatePhaseToGateSpec(spec, phase)
		if spec.Target != GateProfileTargetUnit {
			t.Errorf("Target: got %q, want %q", spec.Target, GateProfileTargetUnit)
		}
		if !spec.Always {
			t.Error("Always: got false, want true")
		}
	})

	t.Run("stack enabled sets StackDetect", func(t *testing.T) {
		t.Parallel()
		stack := &BuildGateStackConfig{Enabled: true, Language: "java", Tool: "maven"}
		spec := &StepGateSpec{}
		phase := &BuildGatePhaseConfig{Stack: stack}
		ApplyBuildGatePhaseToGateSpec(spec, phase)
		if spec.StackDetect != stack {
			t.Errorf("StackDetect: got %v, want stack", spec.StackDetect)
		}
	})

	t.Run("stack disabled does not set StackDetect", func(t *testing.T) {
		t.Parallel()
		stack := &BuildGateStackConfig{Enabled: false, Language: "java", Tool: "maven"}
		spec := &StepGateSpec{}
		phase := &BuildGatePhaseConfig{Stack: stack}
		ApplyBuildGatePhaseToGateSpec(spec, phase)
		if spec.StackDetect != nil {
			t.Errorf("StackDetect: got %v, want nil when stack disabled", spec.StackDetect)
		}
	})

	t.Run("nil stack leaves StackDetect unchanged", func(t *testing.T) {
		t.Parallel()
		existing := &BuildGateStackConfig{Enabled: true, Language: "go"}
		spec := &StepGateSpec{StackDetect: existing}
		phase := &BuildGatePhaseConfig{Target: GateProfileTargetAllTests}
		ApplyBuildGatePhaseToGateSpec(spec, phase)
		if spec.StackDetect != existing {
			t.Error("StackDetect should be unchanged when phase.Stack is nil")
		}
	})

	t.Run("gate_profile propagated", func(t *testing.T) {
		t.Parallel()
		gp := &BuildGateProfileOverride{Command: CommandSpec{Shell: "go test ./..."}}
		spec := &StepGateSpec{}
		phase := &BuildGatePhaseConfig{GateProfile: gp}
		ApplyBuildGatePhaseToGateSpec(spec, phase)
		if spec.GateProfile != gp {
			t.Error("GateProfile: not propagated")
		}
	})
}
