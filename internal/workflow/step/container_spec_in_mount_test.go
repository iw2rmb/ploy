package step

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Verifies that when an inDir is provided, buildContainerSpec mounts it
// at /in and does not alter other mounts.
func TestBuildContainerSpec_InMountPresent(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    types.StepID("step-in-mount"),
		Name:  "With /in",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}

	spec, err := buildContainerSpec(types.RunID("run-in"), types.JobID("job-in"), manifest, "/tmp/ws", "", "/tmp/in", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Expect two mounts: workspace RW and /in.
	if len(spec.Mounts) != 2 {
		t.Fatalf("got %d mounts, want 2: %+v", len(spec.Mounts), spec.Mounts)
	}
	// Find /in mount
	var found bool
	for _, m := range spec.Mounts {
		if m.Target == "/in" {
			found = true
			if m.ReadOnly {
				t.Fatalf("/in mount should be writable: %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("/in mount not present: %+v", spec.Mounts)
	}
}

func TestBuildContainerSpec_InMountSkipsNestedHydraInMounts(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    types.StepID("step-in-mount-hydra"),
		Name:  "With /in and Hydra in",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
		In: []string{"abcdef0:/in/amata.yaml"},
	}

	spec, err := buildContainerSpec(types.RunID("run-in"), types.JobID("job-in"), manifest, "/tmp/ws", "", "/tmp/in", "/tmp/staging")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var foundIn bool
	for _, m := range spec.Mounts {
		if m.Target == "/in" {
			foundIn = true
		}
		if m.Target == "/in/amata.yaml" {
			t.Fatalf("unexpected nested Hydra in mount: %+v", m)
		}
	}
	if !foundIn {
		t.Fatalf("/in mount not present: %+v", spec.Mounts)
	}
}
