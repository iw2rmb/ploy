package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Ensures stage_id and artifact_name options are propagated into manifest.Options
// so that orchestrator uploads can read them.
func TestBuildManifestFromRequest_PropagatesStageAndArtifactName(t *testing.T) {
	req := StartRunRequest{
		RunID:   types.RunID("run-opts-1"),
		RepoURL: types.RepoURL("https://gitlab.com/acme/repo.git"),
		BaseRef: types.GitRef("main"),
		Options: map[string]any{
			"stage_id":      "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			"artifact_name": "custom-bundle",
		},
	}

	m, err := buildManifestFromRequest(req, parseRunOptions(req.Options))
	if err != nil {
		t.Fatalf("buildManifestFromRequest error: %v", err)
	}

	if sid, ok := m.OptionString("stage_id"); !ok || sid == "" {
		t.Fatalf("manifest.Options missing stage_id")
	}
	if an, ok := m.OptionString("artifact_name"); !ok || an != "custom-bundle" {
		t.Fatalf("manifest.Options artifact_name mismatch: got %q", an)
	}
}
