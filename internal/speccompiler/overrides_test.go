package speccompiler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAppliesStepEnvOverridesBeforeEnvironmentExpansion(t *testing.T) {
	repository := t.TempDir()
	specPath := filepath.Join(repository, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(`
steps:
  - name: rewrite
    image: docker.io/test/mig:latest
    envs:
      MODE: $MISSING_MODE
      OTHER: literal
`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	source, err := NewRepositorySource(repository)
	if err != nil {
		t.Fatalf("NewRepositorySource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	compiler, err := New(Options{Source: source})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	payload, err := compiler.Build(context.Background(), "spec.yaml", Overrides{
		StepEnvs: map[string][]string{"rewrite": {"MODE=strict"}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var spec map[string]any
	if err := json.Unmarshal(payload, &spec); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	step := spec["steps"].([]any)[0].(map[string]any)
	envs := step["envs"].(map[string]any)
	if envs["MODE"] != "strict" || envs["OTHER"] != "literal" {
		t.Fatalf("step envs = %#v", envs)
	}
}

func TestBuildStillRejectsNonOverriddenMissingEnvironment(t *testing.T) {
	repository := t.TempDir()
	specPath := filepath.Join(repository, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(`
steps:
  - name: rewrite
    image: docker.io/test/mig:latest
    envs:
      MODE: $MISSING_MODE
`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	source, err := NewRepositorySource(repository)
	if err != nil {
		t.Fatalf("NewRepositorySource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	compiler, err := New(Options{Source: source})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = compiler.Build(context.Background(), "spec.yaml", Overrides{
		StepEnvs: map[string][]string{"rewrite": {"OTHER=value"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved environment variables: MISSING_MODE") {
		t.Fatalf("Build error = %v, want unresolved environment variable", err)
	}
}
