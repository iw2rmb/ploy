package hook

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestLoadFromMigSpec_DirectoryDiscoveryFindsNestedHookYAML(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "hooks", "a", "hook.yaml"), `
id: zeta
steps:
  - image: ghcr.io/example/hook-a:1
`)
	writeFile(t, filepath.Join(root, "hooks", "b", "nested", "hook.yaml"), `
id: alpha
steps:
  - image: ghcr.io/example/hook-b:2
`)
	writeFile(t, filepath.Join(root, "hooks", "b", "nested", "ignore.yaml"), `
id: ignored
steps:
  - image: ghcr.io/example/ignored:0
`)

	spec := contracts.MigSpec{Hooks: []string{"./hooks"}}
	got, err := LoadFromMigSpec(spec, root)
	if err != nil {
		t.Fatalf("LoadFromMigSpec: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(specs) = %d, want 2", len(got))
	}

	wantSources := []string{
		filepath.Join(root, "hooks", "a", "hook.yaml"),
		filepath.Join(root, "hooks", "b", "nested", "hook.yaml"),
	}
	for i := range got {
		if got[i].Source != wantSources[i] {
			t.Fatalf("specs[%d].Source = %q, want %q", i, got[i].Source, wantSources[i])
		}
	}
}

func TestLoadFromMigSpec_LoadsDirectFileAndURLInStableOrder(t *testing.T) {
	root := t.TempDir()
	localPath := filepath.Join(root, "local-hook.yaml")
	localBody, err := os.ReadFile(filepath.Join("testdata", "openapi-generator-cli", "hook.yaml"))
	if err != nil {
		t.Fatalf("read local fixture hook: %v", err)
	}
	writeFile(t, localPath, string(localBody))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/remote/hook.yaml" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`
id: remote-hook
once: false
steps:
  - name: publish
    image: ghcr.io/example/remote:1
`))
	}))
	defer srv.Close()

	sourceURL := srv.URL + "/remote/hook.yaml"
	spec := contracts.MigSpec{
		Hooks: []string{"./local-hook.yaml", sourceURL},
	}

	first, err := LoadFromMigSpec(spec, root)
	if err != nil {
		t.Fatalf("first LoadFromMigSpec: %v", err)
	}
	second, err := LoadFromMigSpec(spec, root)
	if err != nil {
		t.Fatalf("second LoadFromMigSpec: %v", err)
	}

	if len(first) != 2 {
		t.Fatalf("len(first) = %d, want 2", len(first))
	}
	if first[0].ID != "openapi-generator-cli" {
		t.Fatalf("first[0].ID = %q, want %q", first[0].ID, "openapi-generator-cli")
	}
	if first[1].ID != "remote-hook" {
		t.Fatalf("first[1].ID = %q, want %q", first[1].ID, "remote-hook")
	}
	if len(first[0].Steps) != 1 || first[0].Steps[0].Image != "ghcr.io/iw2rmb/hook/openapi-generator-cli:latest" {
		t.Fatalf("local hook steps not preserved: %+v", first[0].Steps)
	}
	if len(first[0].Steps[0].In) != 1 || first[0].Steps[0].In[0] != "abcdef0:/in/amata.yaml" {
		t.Fatalf("local hook step in entries not preserved: %+v", first[0].Steps[0].In)
	}
	if len(first[1].Steps) != 1 || first[1].Steps[0].Name != "publish" {
		t.Fatalf("remote hook steps not preserved: %+v", first[1].Steps)
	}

	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("load order changed at index %d: %q vs %q", i, first[i].ID, second[i].ID)
		}
		if first[i].Source != second[i].Source {
			t.Fatalf("source order changed at index %d: %q vs %q", i, first[i].Source, second[i].Source)
		}
	}
}

func TestLoadFromMigSpec_MalformedSpecsReturnDeterministicErrors(t *testing.T) {
	root := t.TempDir()
	missingIDPath := filepath.Join(root, "hooks", "a", "hook.yaml")
	unknownFieldPath := filepath.Join(root, "hooks", "z", "hook.yaml")

	writeFile(t, missingIDPath, `
steps:
  - image: ghcr.io/example/a:1
`)
	writeFile(t, unknownFieldPath, `
id: bad-field
unknown_key: true
steps:
  - image: ghcr.io/example/z:1
`)

	spec := contracts.MigSpec{Hooks: []string{"./hooks"}}
	_, err := LoadFromMigSpec(spec, root)
	if err == nil {
		t.Fatal("expected error for malformed hooks")
	}

	msg := err.Error()
	if !strings.Contains(msg, missingIDPath) {
		t.Fatalf("error missing first source path %q: %v", missingIDPath, err)
	}
	if !strings.Contains(msg, "id: required") {
		t.Fatalf("error missing required-id validation: %v", err)
	}
	if !strings.Contains(msg, unknownFieldPath) {
		t.Fatalf("error missing second source path %q: %v", unknownFieldPath, err)
	}
	if !strings.Contains(msg, "field unknown_key not found") {
		t.Fatalf("error missing strict unknown-field text: %v", err)
	}
	if strings.Index(msg, missingIDPath) > strings.Index(msg, unknownFieldPath) {
		t.Fatalf("error order is not deterministic: %v", err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(body, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
