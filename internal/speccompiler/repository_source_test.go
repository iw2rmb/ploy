package speccompiler

import (
	"context"
	"encoding/json"
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

// Every mount kind uses the declaring file, even when the root has a same-named source.
func TestRepositoryCompilerReadsMountsFromDeclaringFile(t *testing.T) {
	for _, form := range []string{"include", "nested include", "ref", "ref with include", "included ref"} {
		t.Run(form, func(t *testing.T) {
			repository := t.TempDir()
			if err := os.MkdirAll(filepath.Join(repository, "fragments", "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			step := `name: rewrite
image: docker.io/test/mig:latest
in: [./input.txt:input.txt]
out: [./input.txt:input.txt]
home: [./input.txt:input.txt:ro]
tmp: [./input.txt:input.txt]
`
			files := map[string]string{
				"input.txt":                  "root file must not be selected\n",
				"fragments/input.txt":        "declared source\n",
				"fragments/nested/input.txt": "nested source\n",
				"fragments/step.yaml":        step,
				"fragments/nested/step.yaml": step,
				"fragments/mig.yaml":         "steps:\n  - !include ./step.yaml\n",
			}
			root := "steps:\n  - !include ./fragments/step.yaml\n"
			sourcePath := "fragments/input.txt"
			switch form {
			case "nested include":
				files["fragments/step.yaml"] = "!include ./nested/step.yaml\n"
				sourcePath = "fragments/nested/input.txt"
			case "ref":
				files["fragments/mig.yaml"] = "steps:\n  - " + strings.ReplaceAll(strings.TrimSuffix(step, "\n"), "\n", "\n    ") + "\n"
				root = "steps:\n  - ref: ./fragments/mig.yaml:rewrite\n"
			case "ref with include":
				root = "steps:\n  - ref: ./fragments/mig.yaml:rewrite\n"
			case "included ref":
				files["fragments/wrapper.yaml"] = "ref: ./mig.yaml:rewrite\n"
				root = "steps:\n  - !include ./fragments/wrapper.yaml\n"
			}
			for path, content := range files {
				if err := os.WriteFile(filepath.Join(repository, path), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			source, err := NewRepositorySource(repository)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = source.Close() })
			compiler, err := New(Options{Source: source})
			if err != nil {
				t.Fatal(err)
			}
			payload, err := compiler.Validate([]byte(root), repository)
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			var decoded struct {
				Steps []map[string]json.RawMessage `json:"steps"`
			}
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatal(err)
			}
			hash, err := compiler.localFileRecordHash(sourcePath, "")
			if err != nil {
				t.Fatal(err)
			}
			for kind, destination := range map[string]string{
				"in": "/in/input.txt", "out": "/out/input.txt", "home": "input.txt:ro", "tmp": "/tmp/input.txt",
			} {
				var entries []string
				if err := json.Unmarshal(decoded.Steps[0][kind], &entries); err != nil {
					t.Fatal(err)
				}
				if want := hash + ":" + destination; len(entries) != 1 || entries[0] != want {
					t.Fatalf("%s mounts = %v, want [%s]", kind, entries, want)
				}
			}
		})
	}
}
