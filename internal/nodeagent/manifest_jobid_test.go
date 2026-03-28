package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestBuildManifestFromRequest_StepIDUsesJobID verifies that manifest.ID uses
// JobID when available to ensure uniqueness across jobs within the same run.
// This prevents collisions when multiple jobs (pre_gate, mig, post_gate, heal)
// exist for a single run.
func TestBuildManifestFromRequest_StepIDUsesJobID(t *testing.T) {
	t.Parallel()

	t.Run("uses JobID when provided", func(t *testing.T) {
		t.Parallel()
		req := StartRunRequest{
			RunID:        types.RunID("run-shared-123"),
			JobID:        types.JobID("job-unique-456"),
			RepoURL:      types.RepoURL("https://gitlab.com/acme/repo.git"),
			BaseRef:      types.GitRef("main"),
			TypedOptions: RunOptions{},
		}

		manifest, err := buildManifestFromRequest(req, req.TypedOptions, 0, contracts.MigStackUnknown)
		if err != nil {
			t.Fatalf("buildManifestFromRequest error: %v", err)
		}

		// Manifest ID should be JobID, not RunID.
		if manifest.ID.String() != req.JobID.String() {
			t.Errorf("manifest.ID = %q, want JobID %q", manifest.ID, req.JobID)
		}
	})

	t.Run("requires JobID", func(t *testing.T) {
		t.Parallel()
		req := StartRunRequest{
			RunID:        types.RunID("run-fallback-789"),
			RepoURL:      types.RepoURL("https://gitlab.com/acme/repo.git"),
			BaseRef:      types.GitRef("main"),
			TypedOptions: RunOptions{},
			// JobID intentionally omitted.
		}

		manifest, err := buildManifestFromRequest(req, req.TypedOptions, 0, contracts.MigStackUnknown)
		if err == nil {
			t.Fatalf("expected error when JobID is missing")
		}
		if manifest.ID.String() != "" {
			t.Fatalf("expected empty manifest on error, got ID %q", manifest.ID.String())
		}
	})

	t.Run("different JobIDs produce unique manifest IDs for same RunID", func(t *testing.T) {
		t.Parallel()
		runID := types.RunID("run-multi-job-001")

		// Simulate multiple jobs within the same run (e.g., pre_gate, mig, post_gate).
		jobs := []types.JobID{
			types.JobID("job-pre-gate-001"),
			types.JobID("job-mig-001"),
			types.JobID("job-post-gate-001"),
		}

		manifestIDs := make(map[string]struct{})
		for _, jobID := range jobs {
			req := StartRunRequest{
				RunID:        runID,
				JobID:        jobID,
				RepoURL:      types.RepoURL("https://gitlab.com/acme/repo.git"),
				BaseRef:      types.GitRef("main"),
				TypedOptions: RunOptions{},
			}

			manifest, err := buildManifestFromRequest(req, req.TypedOptions, 0, contracts.MigStackUnknown)
			if err != nil {
				t.Fatalf("buildManifestFromRequest error for job %s: %v", jobID, err)
			}

			id := manifest.ID.String()
			if _, exists := manifestIDs[id]; exists {
				t.Errorf("collision detected: manifest ID %q already used", id)
			}
			manifestIDs[id] = struct{}{}
		}

		// Verify all three jobs produced unique IDs.
		if len(manifestIDs) != len(jobs) {
			t.Errorf("expected %d unique manifest IDs, got %d", len(jobs), len(manifestIDs))
		}
	})
}

// TestBuildGateManifestFromRequest_StepIDUsesJobID verifies that gate manifests
// also use JobID-based IDs for uniqueness.
func TestBuildGateManifestFromRequest_StepIDUsesJobID(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:        types.RunID("run-gate-shared"),
		JobID:        types.JobID("job-gate-unique"),
		RepoURL:      types.RepoURL("https://gitlab.com/acme/repo.git"),
		BaseRef:      types.GitRef("main"),
		TypedOptions: RunOptions{BuildGate: BuildGateOptions{Enabled: true}},
	}

	manifest, err := buildGateManifestFromRequest(req, req.TypedOptions)
	if err != nil {
		t.Fatalf("buildGateManifestFromRequest error: %v", err)
	}

	// Gate manifest ID should use JobID.
	if manifest.ID.String() != req.JobID.String() {
		t.Errorf("gate manifest.ID = %q, want JobID %q", manifest.ID, req.JobID)
	}
}

// TestBuildHealingManifest_StepIDUsesJobID verifies that healing manifests
// use JobID-based IDs for uniqueness across healing jobs.
func TestBuildHealingManifest_StepIDUsesJobID(t *testing.T) {
	t.Parallel()

	t.Run("uses JobID when provided", func(t *testing.T) {
		t.Parallel()
		req := StartRunRequest{
			RunID:   types.RunID("run-heal-shared"),
			JobID:   types.JobID("job-heal-unique"),
			RepoURL: types.RepoURL("https://gitlab.com/acme/repo.git"),
			BaseRef: types.GitRef("main"),
		}

		mig := ModContainerSpec{
			Image: contracts.JobImage{Universal: "healer:latest"},
		}

		manifest, err := buildHealingManifest(req, mig, 0, "", contracts.MigStackUnknown)
		if err != nil {
			t.Fatalf("buildHealingManifest error: %v", err)
		}

		// Healing manifest ID should use JobID with heal suffix.
		wantID := "job-heal-unique-heal-0"
		if manifest.ID.String() != wantID {
			t.Errorf("healing manifest.ID = %q, want %q", manifest.ID, wantID)
		}
	})

	t.Run("requires JobID", func(t *testing.T) {
		t.Parallel()
		req := StartRunRequest{
			RunID:   types.RunID("run-heal-fallback"),
			RepoURL: types.RepoURL("https://gitlab.com/acme/repo.git"),
			BaseRef: types.GitRef("main"),
			// JobID intentionally omitted.
		}

		mig := ModContainerSpec{
			Image: contracts.JobImage{Universal: "healer:latest"},
		}

		_, err := buildHealingManifest(req, mig, 0, "", contracts.MigStackUnknown)
		if err == nil {
			t.Fatalf("expected error when JobID is missing")
		}
	})

	t.Run("different JobIDs produce unique healing manifest IDs", func(t *testing.T) {
		t.Parallel()
		runID := types.RunID("run-heal-multi")

		// Simulate multiple healing jobs within the same run.
		jobs := []types.JobID{
			types.JobID("job-heal-a"),
			types.JobID("job-heal-b"),
			types.JobID("job-heal-c"),
		}

		manifestIDs := make(map[string]struct{})
		for _, jobID := range jobs {
			req := StartRunRequest{
				RunID:   runID,
				JobID:   jobID,
				RepoURL: types.RepoURL("https://gitlab.com/acme/repo.git"),
				BaseRef: types.GitRef("main"),
			}

			mig := ModContainerSpec{
				Image: contracts.JobImage{Universal: "healer:latest"},
			}

			manifest, err := buildHealingManifest(req, mig, 0, "", contracts.MigStackUnknown)
			if err != nil {
				t.Fatalf("buildHealingManifest error for job %s: %v", jobID, err)
			}

			id := manifest.ID.String()
			if _, exists := manifestIDs[id]; exists {
				t.Errorf("collision detected: healing manifest ID %q already used", id)
			}
			manifestIDs[id] = struct{}{}
		}

		// Verify all healing jobs produced unique IDs.
		if len(manifestIDs) != len(jobs) {
			t.Errorf("expected %d unique manifest IDs, got %d", len(jobs), len(manifestIDs))
		}
	})
}

// Ensures job_id and artifact_name options are propagated into manifest.Options
// so that orchestrator uploads can read them.
func TestBuildManifestFromRequest_PropagatesJobAndArtifactName(t *testing.T) {
	req := StartRunRequest{
		RunID:   types.RunID("run-opts-1"),
		JobID:   types.JobID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		RepoURL: types.RepoURL("https://gitlab.com/acme/repo.git"),
		BaseRef: types.GitRef("main"),
		TypedOptions: RunOptions{
			ServerMetadata: ServerMetadataOptions{JobID: types.JobID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")},
			Artifacts:      ArtifactOptions{Name: "custom-bundle"},
		},
	}

	// Pass MigStackUnknown explicitly to indicate tests operate without stack detection.
	m, err := buildManifestFromRequest(req, req.TypedOptions, 0, contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest error: %v", err)
	}

	if jid, ok := m.OptionString("job_id"); !ok || jid == "" {
		t.Fatalf("manifest.Options missing job_id")
	}
	if an, ok := m.OptionString("artifact_name"); !ok || an != "custom-bundle" {
		t.Fatalf("manifest.Options artifact_name mismatch: got %q", an)
	}
}
