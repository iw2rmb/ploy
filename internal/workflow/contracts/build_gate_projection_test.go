package contracts

import "testing"

func TestBuildGateProfileOverrideToSpecMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		override *BuildGateProfileOverride
		assert   func(t *testing.T, m map[string]any)
	}{
		{name: "nil override", override: nil, assert: func(t *testing.T, m map[string]any) {
			t.Helper()
			if m != nil {
				t.Fatalf("map=%v, want nil", m)
			}
		}},
		{name: "shell command", override: &BuildGateProfileOverride{Command: CommandSpec{Shell: "go test ./..."}}, assert: func(t *testing.T, m map[string]any) {
			t.Helper()
			if got, _ := m["command"].(string); got != "go test ./..." {
				t.Fatalf("command=%v", m["command"])
			}
			assertNoKey(t, m, "env")
			assertNoKey(t, m, "stack")
			assertNoKey(t, m, "target")
		}},
		{name: "exec command", override: &BuildGateProfileOverride{Command: CommandSpec{Exec: []string{"go", "test", "./..."}}}, assert: func(t *testing.T, m map[string]any) {
			t.Helper()
			exec, ok := m["command"].([]any)
			if !ok || len(exec) != 3 || exec[0] != "go" || exec[1] != "test" || exec[2] != "./..." {
				t.Fatalf("command=%v", m["command"])
			}
		}},
		{name: "env stack and target", override: &BuildGateProfileOverride{
			Command: CommandSpec{Shell: "mvn test"},
			Env:     map[string]string{"FOO": "bar"},
			Stack:   &GateProfileStack{Language: "java", Tool: "maven", Release: "17"},
			Target:  GateProfileTargetUnit,
		}, assert: func(t *testing.T, m map[string]any) {
			t.Helper()
			env, _ := m["env"].(map[string]any)
			if env["FOO"] != "bar" {
				t.Fatalf("env=%v", env)
			}
			stack, _ := m["stack"].(map[string]any)
			if stack["language"] != "java" || stack["tool"] != "maven" || stack["release"] != "17" {
				t.Fatalf("stack=%v", stack)
			}
			if got, _ := m["target"].(string); got != GateProfileTargetUnit {
				t.Fatalf("target=%q", got)
			}
		}},
		{name: "empty release omitted", override: &BuildGateProfileOverride{
			Command: CommandSpec{Shell: "mvn test"},
			Stack:   &GateProfileStack{Language: "java", Tool: "maven"},
		}, assert: func(t *testing.T, m map[string]any) {
			t.Helper()
			stack, _ := m["stack"].(map[string]any)
			if _, ok := stack["release"]; ok {
				t.Fatalf("stack.release should be omitted: %v", stack)
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := BuildGateProfileOverrideToSpecMap(tc.override)
			tc.assert(t, m)
		})
	}
}

func TestApplyBuildGatePhaseToGateSpec(t *testing.T) {
	t.Parallel()

	t.Run("nil inputs are no-op", func(t *testing.T) {
		t.Parallel()
		spec := &StepGateSpec{Target: "build", Always: true}
		ApplyBuildGatePhaseToGateSpec(spec, nil)
		if spec.Target != "build" || !spec.Always {
			t.Fatalf("spec changed: %+v", spec)
		}
		ApplyBuildGatePhaseToGateSpec(nil, &BuildGatePhaseConfig{Target: GateProfileTargetUnit})
	})

	t.Run("target always gate profile and enabled stack", func(t *testing.T) {
		t.Parallel()
		gp := &BuildGateProfileOverride{Command: CommandSpec{Shell: "go test ./..."}}
		stack := &BuildGateStackConfig{Enabled: true, Language: "java", Tool: "maven"}
		spec := &StepGateSpec{}
		phase := &BuildGatePhaseConfig{Target: GateProfileTargetUnit, Always: true, GateProfile: gp, Stack: stack}
		ApplyBuildGatePhaseToGateSpec(spec, phase)

		if spec.Target != GateProfileTargetUnit || !spec.Always {
			t.Fatalf("target/always mismatch: %+v", spec)
		}
		if spec.GateProfile != gp {
			t.Fatalf("gate profile not propagated")
		}
		if spec.StackDetect != stack {
			t.Fatalf("stack not propagated")
		}
	})

	t.Run("disabled stack is not propagated", func(t *testing.T) {
		t.Parallel()
		existing := &BuildGateStackConfig{Enabled: true, Language: "go"}
		spec := &StepGateSpec{StackDetect: existing}
		phase := &BuildGatePhaseConfig{Stack: &BuildGateStackConfig{Enabled: false, Language: "java", Tool: "maven"}}
		ApplyBuildGatePhaseToGateSpec(spec, phase)
		if spec.StackDetect != existing {
			t.Fatalf("disabled stack should not overwrite existing: %+v", spec.StackDetect)
		}
	})
}

func assertNoKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Fatalf("%s key should be absent: %v", key, m)
	}
}
