package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestBuildManifestFromRequest_StepIDUsesJobID verifies that manifest.ID uses
// JobID when available to ensure uniqueness across jobs within the same run.
func TestBuildManifestFromRequest_StepIDUsesJobID(t *testing.T) {
	t.Parallel()

	t.Run("uses JobID when provided", func(t *testing.T) {
		t.Parallel()
		req := newStartRunRequest(
			withRunID("run-shared-123"), withJobID("job-unique-456"),
		)

		manifest, err := buildManifestDefault(req)
		if err != nil {
			t.Fatalf("buildManifestFromRequest error: %v", err)
		}

		if manifest.ID.String() != req.JobID.String() {
			t.Errorf("manifest.ID = %q, want JobID %q", manifest.ID, req.JobID)
		}
	})

	t.Run("requires JobID", func(t *testing.T) {
		t.Parallel()
		req := newStartRunRequest(
			withRunID("run-fallback-789"), withJobID(""),
		)

		manifest, err := buildManifestDefault(req)
		if err == nil {
			t.Fatalf("expected error when JobID is missing")
		}
		if manifest.ID.String() != "" {
			t.Fatalf("expected empty manifest on error, got ID %q", manifest.ID.String())
		}
	})

	t.Run("different JobIDs produce unique manifest IDs for same RunID", func(t *testing.T) {
		t.Parallel()

		jobs := []types.JobID{
			types.JobID("job-pre-gate-001"),
			types.JobID("job-mig-001"),
			types.JobID("job-post-gate-001"),
		}

		manifestIDs := make(map[string]struct{})
		for _, jobID := range jobs {
			req := newStartRunRequest(
				withRunID("run-multi-job-001"), withJobID(string(jobID)),
			)

			manifest, err := buildManifestDefault(req)
			if err != nil {
				t.Fatalf("buildManifestFromRequest error for job %s: %v", jobID, err)
			}

			id := manifest.ID.String()
			if _, exists := manifestIDs[id]; exists {
				t.Errorf("collision detected: manifest ID %q already used", id)
			}
			manifestIDs[id] = struct{}{}
		}

		if len(manifestIDs) != len(jobs) {
			t.Errorf("expected %d unique manifest IDs, got %d", len(jobs), len(manifestIDs))
		}
	})
}

// TestBuildGateManifestFromRequest_StepIDUsesJobID verifies that gate manifests
// also use JobID-based IDs for uniqueness.
func TestBuildGateManifestFromRequest_StepIDUsesJobID(t *testing.T) {
	t.Parallel()

	req := newStartRunRequest(
		withRunID("run-gate-shared"), withJobID("job-gate-unique"),
		withRunOptions(RunOptions{BuildGate: BuildGateOptions{Enabled: true}}),
	)

	manifest, err := buildGateManifestFromRequest(req, req.TypedOptions)
	if err != nil {
		t.Fatalf("buildGateManifestFromRequest error: %v", err)
	}

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
		req := newStartRunRequest(
			withRunID("run-heal-shared"), withJobID("job-heal-unique"),
		)

		mig := MigContainerSpec{
			Image: contracts.JobImage{Universal: "healer:latest"},
		}

		manifest, err := buildHealingManifest(req, mig, 0, "", contracts.MigStackUnknown)
		if err != nil {
			t.Fatalf("buildHealingManifest error: %v", err)
		}

		wantID := "job-heal-unique-heal-0"
		if manifest.ID.String() != wantID {
			t.Errorf("healing manifest.ID = %q, want %q", manifest.ID, wantID)
		}
	})

	t.Run("requires JobID", func(t *testing.T) {
		t.Parallel()
		req := newStartRunRequest(
			withRunID("run-heal-fallback"), withJobID(""),
		)

		mig := MigContainerSpec{
			Image: contracts.JobImage{Universal: "healer:latest"},
		}

		_, err := buildHealingManifest(req, mig, 0, "", contracts.MigStackUnknown)
		if err == nil {
			t.Fatalf("expected error when JobID is missing")
		}
	})

	t.Run("different JobIDs produce unique healing manifest IDs", func(t *testing.T) {
		t.Parallel()

		jobs := []types.JobID{
			types.JobID("job-heal-a"),
			types.JobID("job-heal-b"),
			types.JobID("job-heal-c"),
		}

		manifestIDs := make(map[string]struct{})
		for _, jobID := range jobs {
			req := newStartRunRequest(
				withRunID("run-heal-multi"), withJobID(string(jobID)),
			)

			mig := MigContainerSpec{
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

		if len(manifestIDs) != len(jobs) {
			t.Errorf("expected %d unique manifest IDs, got %d", len(jobs), len(manifestIDs))
		}
	})
}

// TestBuildManifestFromRequest_PropagatesJobAndArtifactName ensures job_id and
// artifact_name options are propagated into manifest.Options.
func TestBuildManifestFromRequest_PropagatesJobAndArtifactName(t *testing.T) {
	req := newStartRunRequest(
		withRunID("run-opts-1"),
		withJobID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		withRunOptions(RunOptions{
			ServerMetadata: ServerMetadataOptions{JobID: types.JobID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")},
			Artifacts:      ArtifactOptions{Name: "custom-bundle"},
		}),
	)

	m, err := buildManifestDefault(req)
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
