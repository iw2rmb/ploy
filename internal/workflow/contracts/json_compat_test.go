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
		RunID:         types.RunID("run-123"),
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
		Repo: RepoMaterialization{
			URL:           types.RepoURL("https://gitlab.com/iw2rmb/sample.git"),
			BaseRef:       types.GitRef("main"),
			TargetRef:     types.GitRef("mods/shift-grid"),
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

func TestJSONCompatibility_WorkflowCheckpoint_Golden(t *testing.T) {
	cp := WorkflowCheckpoint{
		SchemaVersion: SchemaVersion,
		RunID:         types.RunID("run-123"),
		Stage:         StageName("mods-plan"),
		Status:        CheckpointStatusPending,
		CacheKey:      "node-wasm/cache@manifest=2025-09-26@aster=plan",
		StageMetadata: &CheckpointStage{
			Name:         "mods-plan",
			Kind:         "mods-plan",
			Lane:         "node-wasm",
			Dependencies: []string{"compile"},
			Manifest:     ManifestReference{Name: "smoke", Version: "2025-09-26"},
			Aster: CheckpointStageAster{
				Enabled: true,
				Toggles: []string{"plan"},
				Bundles: []CheckpointAsterBundle{{
					Stage:       "mods-plan",
					Toggle:      "plan",
					BundleID:    "mods-plan",
					Digest:      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					ArtifactCID: "cid-mods-plan",
				}},
			},
		},
		Artifacts: []CheckpointArtifact{{
			Name:        "mods-plan-bundle",
			ArtifactCID: types.CID("cid-mods-plan"),
			Digest:      types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			MediaType:   "application/tar+zst",
		}},
	}
	got, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("marshal checkpoint: %v", err)
	}
	want := loadGolden(t, "checkpoint_golden.json")
	if !reflect.DeepEqual(jsonAsInterface(t, got), jsonAsInterface(t, want)) {
		t.Fatalf("checkpoint json mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestJSONCompatibility_WorkflowArtifact_Golden(t *testing.T) {
	env := WorkflowArtifact{
		SchemaVersion: SchemaVersion,
		RunID:         types.RunID("run-123"),
		Stage:         StageName("mods-plan"),
		CacheKey:      "node-wasm/cache@manifest=2025-09-26@aster=plan",
		StageMetadata: &CheckpointStage{
			Name:     "mods-plan",
			Kind:     "mods-plan",
			Lane:     "node-wasm",
			Manifest: ManifestReference{Name: "smoke", Version: "2025-09-26"},
		},
		Artifact: CheckpointArtifact{
			Name:        "mods-plan",
			ArtifactCID: types.CID("cid-mods-plan"),
			Digest:      types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			MediaType:   "application/tar+zst",
		},
	}
	got, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}
	want := loadGolden(t, "artifact_golden.json")
	if !reflect.DeepEqual(jsonAsInterface(t, got), jsonAsInterface(t, want)) {
		t.Fatalf("artifact json mismatch\n got: %s\nwant: %s", got, want)
	}
}
