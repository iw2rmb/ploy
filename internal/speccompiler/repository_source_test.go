package speccompiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositorySourceRejectsNonRepositoryPaths(t *testing.T) {
	parent := t.TempDir()
	repository := filepath.Join(parent, "repo")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	source, err := NewRepositorySource(repository)
	if err != nil {
		t.Fatalf("NewRepositorySource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })

	for _, path := range []string{"../outside.yaml", "/tmp/outside.yaml", "~/outside.yaml", "$OUTSIDE/spec.yaml"} {
		t.Run(path, func(t *testing.T) {
			if _, err := source.ResolveReference(path, repository); err == nil {
				t.Fatalf("ResolveReference(%q) succeeded, want repository-boundary error", path)
			}
		})
	}
}

func TestRepositoryCompilerRejectsTraversalAndSymlinkEscapes(t *testing.T) {
	parent := t.TempDir()
	repository := filepath.Join(parent, "repo")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	outside := filepath.Join(parent, "outside.yaml")
	if err := os.WriteFile(outside, []byte("image: docker.io/test/mig:latest\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink("../outside.yaml", filepath.Join(repository, "outside-link.yaml")); err != nil {
		t.Fatalf("create outside symlink: %v", err)
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

	tests := []struct {
		name string
		spec string
	}{
		{
			name: "include traversal",
			spec: "steps:\n  - !include ../outside.yaml\n",
		},
		{
			name: "include symlink",
			spec: "steps:\n  - !include ./outside-link.yaml\n",
		},
		{
			name: "ref traversal",
			spec: "steps:\n  - ref: ../outside.yaml:rewrite\n",
		},
		{
			name: "file record traversal",
			spec: "steps:\n  - image: docker.io/test/mig:latest\n    in:\n      - ../outside.yaml:outside.yaml\n",
		},
		{
			name: "file record symlink",
			spec: "steps:\n  - image: docker.io/test/mig:latest\n    in:\n      - ./outside-link.yaml:outside.yaml\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := compiler.Validate([]byte(test.spec), repository); err == nil {
				t.Fatal("Validate succeeded, want repository-boundary error")
			}
		})
	}
}

func TestRepositoryCompilerDoesNotReadAmbientEnvironment(t *testing.T) {
	repository := t.TempDir()
	specPath := filepath.Join(repository, "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: $SERVER_ONLY_IMAGE\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	t.Setenv("SERVER_ONLY_IMAGE", "docker.io/private/server-only:latest")

	source, err := NewRepositorySource(repository)
	if err != nil {
		t.Fatalf("NewRepositorySource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	compiler, err := New(Options{Source: source})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = compiler.Build(context.Background(), "spec.yaml", Overrides{})
	if err == nil || !strings.Contains(err.Error(), "unresolved environment variables: SERVER_ONLY_IMAGE") {
		t.Fatalf("Build error = %v, want unresolved environment variable", err)
	}
}

func TestRepositoryCompilerReadsIncludedFilesWithinRoot(t *testing.T) {
	repository := t.TempDir()
	fragmentDir := filepath.Join(repository, "fragments")
	if err := os.Mkdir(fragmentDir, 0o755); err != nil {
		t.Fatalf("create fragment directory: %v", err)
	}
	files := map[string]string{
		filepath.Join(repository, "spec.yaml"):  "steps:\n  - !include ./fragments/step.yaml\n",
		filepath.Join(fragmentDir, "step.yaml"): "image: docker.io/test/mig:latest\nin:\n  - ./input.txt:input.txt\n",
		filepath.Join(fragmentDir, "input.txt"): "input\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", filepath.Base(path), err)
		}
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

	payload, err := compiler.Validate([]byte(files[filepath.Join(repository, "spec.yaml")]), repository)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.Contains(string(payload), `:/in/input.txt`) {
		t.Fatalf("payload = %s, want canonical included input", payload)
	}
}
