package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func loadGolden(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("testdata", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read golden %s: %v", p, err)
	}
	return b
}

func jsonAsInterface(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return v
}

func TestJSONCompatibility_WorkflowRun_Golden(t *testing.T) {
	run := WorkflowRun{
		SchemaVersion: SchemaVersion,
		RunID:         types.RunID("123456789012345678901234567"),
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
		Repo: RepoMaterialization{
			URL:           types.RepoURL("https://gitlab.com/iw2rmb/sample.git"),
			BaseRef:       types.GitRef("main"),
			TargetRef:     types.GitRef("migs/example-grid"),
			Commit:        types.CommitSHA("abcdef1234567890"),
			WorkspaceHint: "ws",
		},
	}
	got, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal run envelope: %v", err)
	}
	want := loadGolden(t, "run_golden.json")
	if !reflect.DeepEqual(jsonAsInterface(t, got), jsonAsInterface(t, want)) {
		t.Fatalf("run json mismatch\n got: %s\nwant: %s", got, want)
	}
}
