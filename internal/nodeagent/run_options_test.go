package nodeagent

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestParseSpec_ProducesTypedOptions(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"steps": [{
			"image": "docker.io/test/mig:latest",
			"command": "run-test.sh"
		}],
		"build_gate": {
			"enabled": false
		},
		"gitlab_pat": "glpat-secret",
		"mr_on_success": true,
		"job_id": "` + testKSUID + `",
		"artifact_name": "bundle.tar.gz",
		"artifact_paths": ["a.txt", "b/"]
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	resolved, err := typedOpts.Execution.Image.ResolveImage(contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("unexpected error resolving image: %v", err)
	}
	if resolved != "docker.io/test/mig:latest" {
		t.Errorf("expected typed image=docker.io/test/mig:latest, got %q", resolved)
	}
	if typedOpts.Execution.Command.Shell != "run-test.sh" {
		t.Errorf("expected typed command.shell=run-test.sh, got %q", typedOpts.Execution.Command.Shell)
	}
	if typedOpts.BuildGate.Enabled {
		t.Errorf("expected typed build_gate.enabled=false")
	}
	if typedOpts.MRWiring.GitLabPAT != "glpat-secret" {
		t.Errorf("expected typed gitlab_pat=glpat-secret, got %q", typedOpts.MRWiring.GitLabPAT)
	}
	if !typedOpts.MRFlagsPresent.MROnSuccessSet || !typedOpts.MRWiring.MROnSuccess {
		t.Errorf("expected typed mr_on_success=true and present")
	}
	if typedOpts.ServerMetadata.JobID.String() != testKSUID {
		t.Errorf("expected typed job_id=%s, got %q", testKSUID, typedOpts.ServerMetadata.JobID.String())
	}
	if typedOpts.Artifacts.Name != "bundle.tar.gz" {
		t.Errorf("expected typed artifact_name=bundle.tar.gz, got %q", typedOpts.Artifacts.Name)
	}
	if len(typedOpts.Artifacts.Paths) != 2 {
		t.Fatalf("expected 2 artifact_paths, got %d", len(typedOpts.Artifacts.Paths))
	}
}

func TestParseSpec_EnvMergingSemantics(t *testing.T) {
	t.Parallel()

	t.Run("single_step_merges_step_env", func(t *testing.T) {
		specJSON := `{
			"env": {"A":"1","B":"2"},
			"steps": [{
				"image": "img",
				"env": {"B":"step","C":"3"}
			}]
		}`

		var raw json.RawMessage = []byte(specJSON)
		env, _, _ := parseSpec(raw)

		if env["A"] != "1" || env["B"] != "step" || env["C"] != "3" {
			t.Fatalf("env merge mismatch: got %+v", env)
		}
	})

	t.Run("multi_step_returns_global_env_only", func(t *testing.T) {
		specJSON := `{
			"env": {"A":"1"},
			"steps": [
				{"image":"a","env":{"A":"step0","B":"0"}},
				{"image":"b","env":{"A":"step1","B":"1"}}
			]
		}`

		var raw json.RawMessage = []byte(specJSON)
		env, typedOpts, _ := parseSpec(raw)

		if env["A"] != "1" || len(env) != 1 {
			t.Fatalf("env should contain only global env for multi-step, got %+v", env)
		}
		if len(typedOpts.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(typedOpts.Steps))
		}
		if typedOpts.Steps[0].Env["A"] != "step0" || typedOpts.Steps[1].Env["A"] != "step1" {
			t.Fatalf("expected per-step env in typed options, got step0=%+v step1=%+v", typedOpts.Steps[0].Env, typedOpts.Steps[1].Env)
		}
	})
}

func TestParseSpec_ModIndexRejected(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"mod_index": 1,
		"steps": [
			{"image":"docker.io/test/step-a:v1"},
			{"image":"docker.io/test/step-b:v1"}
		]
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	if len(typedOpts.Steps) != 0 {
		t.Fatalf("expected mod_index to be rejected (zero typed options), got steps_len=%d", len(typedOpts.Steps))
	}
	if !typedOpts.Execution.Image.IsEmpty() {
		t.Fatalf("expected mod_index to be rejected (zero typed options), got execution.image=%v", typedOpts.Execution.Image)
	}
}

func TestParseSpec_ImageMap_PopulatesExecutionImage(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"steps": [{
			"image": {
				"default": "docker.io/user/migs-orw:latest",
				"java-maven": "docker.io/user/orw-cli:latest",
				"java-gradle": "docker.io/user/orw-cli:latest"
			}
		}]
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	mavenImg, err := typedOpts.Execution.Image.ResolveImage(contracts.ModStackJavaMaven)
	if err != nil {
		t.Fatalf("unexpected error resolving maven image: %v", err)
	}
	if mavenImg != "docker.io/user/orw-cli:latest" {
		t.Errorf("expected maven image, got %q", mavenImg)
	}
}

func TestCommand_ToSlice(t *testing.T) {
	t.Parallel()

	t.Run("shell command", func(t *testing.T) {
		cmd := contracts.CommandSpec{Shell: "echo test"}
		result := cmd.ToSlice()
		want := []string{"/bin/sh", "-c", "echo test"}
		if len(result) != len(want) {
			t.Fatalf("expected length=%d, got %d", len(want), len(result))
		}
		for i, v := range want {
			if result[i] != v {
				t.Errorf("expected result[%d]=%q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("exec array", func(t *testing.T) {
		cmd := contracts.CommandSpec{Exec: []string{"/bin/ls", "-la"}}
		result := cmd.ToSlice()
		want := []string{"/bin/ls", "-la"}
		if len(result) != len(want) {
			t.Fatalf("expected length=%d, got %d", len(want), len(result))
		}
		for i, v := range want {
			if result[i] != v {
				t.Errorf("expected result[%d]=%q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("empty command", func(t *testing.T) {
		cmd := contracts.CommandSpec{}
		result := cmd.ToSlice()
		if result != nil {
			t.Errorf("expected nil for empty command, got %v", result)
		}
	})

	t.Run("exec takes precedence over shell", func(t *testing.T) {
		cmd := contracts.CommandSpec{
			Shell: "echo shell",
			Exec:  []string{"/bin/exec"},
		}
		result := cmd.ToSlice()
		if len(result) != 1 || result[0] != "/bin/exec" {
			t.Errorf("expected exec to take precedence, got %v", result)
		}
	})
}

func TestModsSpecToRunOptions_DirectConversion(t *testing.T) {
	t.Parallel()

	t.Run("single_step_with_all_options", func(t *testing.T) {
		t.Parallel()

		mrOnSuccess := true
		mrOnFail := false

		spec := &contracts.ModsSpec{
			JobID: "job-direct-test-123",
			Steps: []contracts.ModStep{
				{
					Image:   contracts.JobImage{Universal: "docker.io/test/mig:v1"},
					Command: contracts.CommandSpec{Exec: []string{"echo", "hello"}},
					Env:     map[string]string{"KEY": "value"},
				},
			},
			BuildGate: &contracts.BuildGateConfig{
				Enabled: true,
				Pre: &contracts.BuildGatePhaseConfig{
					Target: contracts.GateProfileTargetUnit,
					Always: true,
					GateProfile: &contracts.BuildGateProfileOverride{
						Command: contracts.CommandSpec{Shell: "go test ./..."},
						Env:     map[string]string{"GOFLAGS": "-mod=readonly"},
					},
				},
				Post: &contracts.BuildGatePhaseConfig{
					Target: contracts.GateProfileTargetAllTests,
					Always: false,
					GateProfile: &contracts.BuildGateProfileOverride{
						Command: contracts.CommandSpec{Exec: []string{"go", "test", "./...", "-run", "TestUnit"}},
						Env:     map[string]string{"CGO_ENABLED": "0"},
					},
				},
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "infra",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"infra": {
							Retries: 3,
							Image:   contracts.JobImage{Universal: "docker.io/test/heal:v1"},
							Command: contracts.CommandSpec{Shell: "fix.sh"},
							Env:     map[string]string{"MODE": "auto"},
						},
					},
				},
			},
			GitLabPAT:     "glpat-secret",
			GitLabDomain:  "gitlab.example.com",
			MROnSuccess:   &mrOnSuccess,
			MROnFail:      &mrOnFail,
			ArtifactPaths: []string{"path/to/file.txt", "path/to/dir/"},
			ArtifactName:  "my-artifact",
		}

		runOpts := modsSpecToRunOptions(spec)

		if runOpts.ServerMetadata.JobID.String() != "job-direct-test-123" {
			t.Errorf("JobID: got %q, want %q", runOpts.ServerMetadata.JobID.String(), "job-direct-test-123")
		}

		execImg, err := runOpts.Execution.Image.ResolveImage(contracts.ModStackUnknown)
		if err != nil {
			t.Fatalf("unexpected image resolve error: %v", err)
		}
		if execImg != "docker.io/test/mig:v1" {
			t.Errorf("Execution.Image: got %q, want %q", execImg, "docker.io/test/mig:v1")
		}
		wantExec := []string{"echo", "hello"}
		if len(runOpts.Execution.Command.Exec) != len(wantExec) {
			t.Fatalf("Execution.Command.Exec length: got %d, want %d", len(runOpts.Execution.Command.Exec), len(wantExec))
		}
		for i, v := range wantExec {
			if runOpts.Execution.Command.Exec[i] != v {
				t.Errorf("Execution.Command.Exec[%d]: got %q, want %q", i, runOpts.Execution.Command.Exec[i], v)
			}
		}
		if !runOpts.BuildGate.Enabled {
			t.Error("BuildGate.Enabled: expected true")
		}
		if runOpts.BuildGate.PreGateProfile == nil {
			t.Fatal("BuildGate.PreGateProfile: expected non-nil")
		}
		if runOpts.BuildGate.PreGateProfile.Command.Shell != "go test ./..." {
			t.Errorf("BuildGate.PreGateProfile.Command.Shell: got %q, want %q", runOpts.BuildGate.PreGateProfile.Command.Shell, "go test ./...")
		}
		if got := runOpts.BuildGate.PreGateProfile.Env["GOFLAGS"]; got != "-mod=readonly" {
			t.Errorf("BuildGate.PreGateProfile.Env[GOFLAGS]: got %q, want %q", got, "-mod=readonly")
		}
		if got := runOpts.BuildGate.PreTarget; got != contracts.GateProfileTargetUnit {
			t.Errorf("BuildGate.PreTarget: got %q, want %q", got, contracts.GateProfileTargetUnit)
		}
		if !runOpts.BuildGate.PreAlways {
			t.Error("BuildGate.PreAlways: got false, want true")
		}
		if runOpts.BuildGate.PostGateProfile == nil {
			t.Fatal("BuildGate.PostGateProfile: expected non-nil")
		}
		wantPost := []string{"go", "test", "./...", "-run", "TestUnit"}
		if len(runOpts.BuildGate.PostGateProfile.Command.Exec) != len(wantPost) {
			t.Fatalf("BuildGate.PostGateProfile.Command.Exec length: got %d, want %d", len(runOpts.BuildGate.PostGateProfile.Command.Exec), len(wantPost))
		}
		for i, v := range wantPost {
			if got := runOpts.BuildGate.PostGateProfile.Command.Exec[i]; got != v {
				t.Errorf("BuildGate.PostGateProfile.Command.Exec[%d]: got %q, want %q", i, got, v)
			}
		}
		if got := runOpts.BuildGate.PostGateProfile.Env["CGO_ENABLED"]; got != "0" {
			t.Errorf("BuildGate.PostGateProfile.Env[CGO_ENABLED]: got %q, want %q", got, "0")
		}
		if got := runOpts.BuildGate.PostTarget; got != contracts.GateProfileTargetAllTests {
			t.Errorf("BuildGate.PostTarget: got %q, want %q", got, contracts.GateProfileTargetAllTests)
		}
		if runOpts.BuildGate.PostAlways {
			t.Error("BuildGate.PostAlways: got true, want false")
		}

		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if runOpts.Healing.Retries != 3 {
			t.Errorf("Healing.Retries: got %d, want 3", runOpts.Healing.Retries)
		}
		healImg, err := runOpts.Healing.Mod.Image.ResolveImage(contracts.ModStackUnknown)
		if err != nil {
			t.Fatalf("unexpected healing image resolve error: %v", err)
		}
		if healImg != "docker.io/test/heal:v1" {
			t.Errorf("Healing.Mod.Image: got %q, want %q", healImg, "docker.io/test/heal:v1")
		}

		if runOpts.MRWiring.GitLabPAT != "glpat-secret" {
			t.Errorf("MRWiring.GitLabPAT: got %q, want glpat-secret", runOpts.MRWiring.GitLabPAT)
		}
		if !runOpts.MRFlagsPresent.MROnSuccessSet || !runOpts.MRWiring.MROnSuccess {
			t.Errorf("expected mr_on_success present and true")
		}
		if !runOpts.MRFlagsPresent.MROnFailSet || runOpts.MRWiring.MROnFail {
			t.Errorf("expected mr_on_fail present and false")
		}

		if runOpts.Artifacts.Name != "my-artifact" {
			t.Errorf("Artifacts.Name: got %q, want my-artifact", runOpts.Artifacts.Name)
		}
		if len(runOpts.Artifacts.Paths) != 2 {
			t.Errorf("Artifacts.Paths: expected 2, got %d", len(runOpts.Artifacts.Paths))
		}

		if len(runOpts.Steps) != 0 {
			t.Errorf("Steps: expected empty for single-step spec, got %d", len(runOpts.Steps))
		}
	})

	t.Run("multi_step_spec", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{
				{
					Image:   contracts.JobImage{Universal: "docker.io/test/step1:v1"},
					Command: contracts.CommandSpec{Shell: "step1.sh"},
					Env:     map[string]string{"STEP": "1"},
				},
				{
					Image:   contracts.JobImage{Universal: "docker.io/test/step2:v1"},
					Command: contracts.CommandSpec{Exec: []string{"step2", "--flag"}},
					Env:     map[string]string{"STEP": "2"},
				},
			},
		}

		runOpts := modsSpecToRunOptions(spec)

		if len(runOpts.Steps) != 2 {
			t.Fatalf("Steps: expected 2, got %d", len(runOpts.Steps))
		}

		step0Img, err := runOpts.Steps[0].Image.ResolveImage(contracts.ModStackUnknown)
		if err != nil {
			t.Fatalf("unexpected step0 image error: %v", err)
		}
		if step0Img != "docker.io/test/step1:v1" {
			t.Errorf("Steps[0].Image: got %q, want %q", step0Img, "docker.io/test/step1:v1")
		}
		if runOpts.Steps[0].Command.Shell != "step1.sh" {
			t.Errorf("Steps[0].Command.Shell: got %q, want %q", runOpts.Steps[0].Command.Shell, "step1.sh")
		}
		if runOpts.Steps[0].Env["STEP"] != "1" {
			t.Errorf("Steps[0].Env[STEP]: got %q, want %q", runOpts.Steps[0].Env["STEP"], "1")
		}

		step1Img, err := runOpts.Steps[1].Image.ResolveImage(contracts.ModStackUnknown)
		if err != nil {
			t.Fatalf("unexpected step1 image error: %v", err)
		}
		if step1Img != "docker.io/test/step2:v1" {
			t.Errorf("Steps[1].Image: got %q, want %q", step1Img, "docker.io/test/step2:v1")
		}
		wantExec := []string{"step2", "--flag"}
		if len(runOpts.Steps[1].Command.Exec) != len(wantExec) {
			t.Fatalf("Steps[1].Command.Exec length: got %d, want %d", len(runOpts.Steps[1].Command.Exec), len(wantExec))
		}
		for i, v := range wantExec {
			if runOpts.Steps[1].Command.Exec[i] != v {
				t.Errorf("Steps[1].Command.Exec[%d]: got %q, want %q", i, runOpts.Steps[1].Command.Exec[i], v)
			}
		}
		if !runOpts.Execution.Image.IsEmpty() {
			t.Errorf("Execution.Image: expected empty for multi-step spec")
		}
	})

	t.Run("nil_spec_returns_zero_value", func(t *testing.T) {
		t.Parallel()

		runOpts := modsSpecToRunOptions(nil)
		if !runOpts.Execution.Image.IsEmpty() {
			t.Error("expected empty Execution.Image for nil spec")
		}
		if runOpts.Healing != nil {
			t.Error("expected nil Healing for nil spec")
		}
	})

	t.Run("healing_retries_defaults_to_1", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
			BuildGate: &contracts.BuildGateConfig{
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "infra",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"infra": {
							Retries: 0,
							Image:   contracts.JobImage{Universal: "heal"},
						},
					},
				},
			},
		}

		runOpts := modsSpecToRunOptions(spec)

		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if runOpts.Healing.Retries != 1 {
			t.Errorf("Healing.Retries: got %d, want 1 (default)", runOpts.Healing.Retries)
		}
	})

	t.Run("stack_aware_image_preserved", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{
				{
					Image: contracts.JobImage{
						ByStack: map[contracts.ModStack]string{
							contracts.ModStackDefault:    "docker.io/test/default:v1",
							contracts.ModStackJavaMaven:  "docker.io/test/maven:v1",
							contracts.ModStackJavaGradle: "docker.io/test/gradle:v1",
						},
					},
				},
			},
		}

		runOpts := modsSpecToRunOptions(spec)

		mavenImg, err := runOpts.Execution.Image.ResolveImage(contracts.ModStackJavaMaven)
		if err != nil {
			t.Fatalf("unexpected maven image error: %v", err)
		}
		if mavenImg != "docker.io/test/maven:v1" {
			t.Errorf("Maven image: got %q, want %q", mavenImg, "docker.io/test/maven:v1")
		}
	})
}

func TestModsSpecToRunOptions_TmpDir(t *testing.T) {
	t.Parallel()

	payload := []contracts.TmpFilePayload{
		{Name: "config.json", Content: []byte(`{"key":"value"}`)},
		{Name: "secret.txt", Content: []byte("secret")},
	}

	t.Run("single_step_tmpdir_copied", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{
				{
					Image:  contracts.JobImage{Universal: "img"},
					TmpDir: payload,
				},
			},
		}
		runOpts := modsSpecToRunOptions(spec)

		if len(runOpts.Execution.TmpDir) != len(payload) {
			t.Fatalf("Execution.TmpDir len: got %d, want %d", len(runOpts.Execution.TmpDir), len(payload))
		}
		if runOpts.Execution.TmpDir[0].Name != "config.json" {
			t.Errorf("Execution.TmpDir[0].Name: got %q, want config.json", runOpts.Execution.TmpDir[0].Name)
		}
		// Ensure it's a copy, not the same slice.
		if &runOpts.Execution.TmpDir[0] == &payload[0] {
			t.Error("Execution.TmpDir must be a copy, not the original slice header")
		}
	})

	t.Run("multi_step_tmpdir_copied_per_step", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{
				{
					Image:  contracts.JobImage{Universal: "img1"},
					TmpDir: payload,
				},
				{
					Image: contracts.JobImage{Universal: "img2"},
				},
			},
		}
		runOpts := modsSpecToRunOptions(spec)

		if len(runOpts.Steps) != 2 {
			t.Fatalf("Steps len: got %d, want 2", len(runOpts.Steps))
		}
		if len(runOpts.Steps[0].TmpDir) != len(payload) {
			t.Fatalf("Steps[0].TmpDir len: got %d, want %d", len(runOpts.Steps[0].TmpDir), len(payload))
		}
		if runOpts.Steps[0].TmpDir[0].Name != "config.json" {
			t.Errorf("Steps[0].TmpDir[0].Name: got %q, want config.json", runOpts.Steps[0].TmpDir[0].Name)
		}
		if len(runOpts.Steps[1].TmpDir) != 0 {
			t.Errorf("Steps[1].TmpDir: got len %d, want 0", len(runOpts.Steps[1].TmpDir))
		}
	})

	t.Run("healing_tmpdir_copied", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
			BuildGate: &contracts.BuildGateConfig{
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "code",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"code": {
							Image:  contracts.JobImage{Universal: "heal-img"},
							TmpDir: payload,
						},
					},
				},
				Router: &contracts.RouterSpec{
					Image: contracts.JobImage{Universal: "router-img"},
				},
			},
		}
		runOpts := modsSpecToRunOptions(spec)

		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if len(runOpts.Healing.Mod.TmpDir) != len(payload) {
			t.Fatalf("Healing.Mod.TmpDir len: got %d, want %d", len(runOpts.Healing.Mod.TmpDir), len(payload))
		}
		if runOpts.Healing.Mod.TmpDir[0].Name != "config.json" {
			t.Errorf("Healing.Mod.TmpDir[0].Name: got %q, want config.json", runOpts.Healing.Mod.TmpDir[0].Name)
		}
	})

	t.Run("router_tmpdir_copied", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
			BuildGate: &contracts.BuildGateConfig{
				Router: &contracts.RouterSpec{
					Image:  contracts.JobImage{Universal: "router-img"},
					TmpDir: payload,
				},
			},
		}
		runOpts := modsSpecToRunOptions(spec)

		if runOpts.Router == nil {
			t.Fatal("expected Router config")
		}
		if len(runOpts.Router.TmpDir) != len(payload) {
			t.Fatalf("Router.TmpDir len: got %d, want %d", len(runOpts.Router.TmpDir), len(payload))
		}
		if runOpts.Router.TmpDir[0].Name != "config.json" {
			t.Errorf("Router.TmpDir[0].Name: got %q, want config.json", runOpts.Router.TmpDir[0].Name)
		}
	})
}

func TestModsSpecToRunOptions_Amata(t *testing.T) {
	t.Parallel()

	amataSpec := &contracts.AmataRunSpec{
		Spec: "task: fix-it\nprompt: fix the bug",
		Set: []contracts.AmataSetParam{
			{Param: "repo", Value: "myrepo"},
			{Param: "env", Value: "prod"},
		},
	}

	t.Run("router_amata_propagated", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
			BuildGate: &contracts.BuildGateConfig{
				Router: &contracts.RouterSpec{
					Image: contracts.JobImage{Universal: "router-img"},
					Amata: amataSpec,
				},
			},
		}
		runOpts := modsSpecToRunOptions(spec)

		if runOpts.Router == nil {
			t.Fatal("expected Router config")
		}
		if runOpts.Router.Amata == nil {
			t.Fatal("Router.Amata: expected non-nil")
		}
		if runOpts.Router.Amata.Spec != amataSpec.Spec {
			t.Errorf("Router.Amata.Spec: got %q, want %q", runOpts.Router.Amata.Spec, amataSpec.Spec)
		}
		if len(runOpts.Router.Amata.Set) != 2 {
			t.Fatalf("Router.Amata.Set len: got %d, want 2", len(runOpts.Router.Amata.Set))
		}
		if runOpts.Router.Amata.Set[0].Param != "repo" || runOpts.Router.Amata.Set[1].Param != "env" {
			t.Errorf("Router.Amata.Set order: got %v", runOpts.Router.Amata.Set)
		}
	})

	t.Run("healing_amata_propagated", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
			BuildGate: &contracts.BuildGateConfig{
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "code",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"code": {
							Image: contracts.JobImage{Universal: "heal-img"},
							Amata: amataSpec,
						},
					},
				},
			},
		}
		runOpts := modsSpecToRunOptions(spec)

		if runOpts.Healing == nil {
			t.Fatal("expected Healing config")
		}
		if runOpts.Healing.Mod.Amata == nil {
			t.Fatal("Healing.Mod.Amata: expected non-nil")
		}
		if runOpts.Healing.Mod.Amata.Spec != amataSpec.Spec {
			t.Errorf("Healing.Mod.Amata.Spec: got %q, want %q", runOpts.Healing.Mod.Amata.Spec, amataSpec.Spec)
		}
		if len(runOpts.Healing.Mod.Amata.Set) != 2 {
			t.Fatalf("Healing.Mod.Amata.Set len: got %d, want 2", len(runOpts.Healing.Mod.Amata.Set))
		}
	})

	t.Run("nil_amata_propagates_nil", func(t *testing.T) {
		t.Parallel()

		spec := &contracts.ModsSpec{
			Steps: []contracts.ModStep{{Image: contracts.JobImage{Universal: "img"}}},
			BuildGate: &contracts.BuildGateConfig{
				Router: &contracts.RouterSpec{
					Image: contracts.JobImage{Universal: "router-img"},
				},
				Healing: &contracts.HealingSpec{
					SelectedErrorKind: "code",
					ByErrorKind: map[string]contracts.HealingActionSpec{
						"code": {Image: contracts.JobImage{Universal: "heal-img"}},
					},
				},
			},
		}
		runOpts := modsSpecToRunOptions(spec)

		if runOpts.Router != nil && runOpts.Router.Amata != nil {
			t.Error("Router.Amata: expected nil when not configured")
		}
		if runOpts.Healing != nil && runOpts.Healing.Mod.Amata != nil {
			t.Error("Healing.Mod.Amata: expected nil when not configured")
		}
	})
}
