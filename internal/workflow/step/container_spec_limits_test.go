package step

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildContainerSpec_ResourceLimitsApplied(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    types.StepID("step-limits"),
		Name:  "With Limits",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadOnly,
			SnapshotCID: types.CID("bafy123"),
		}},
		Resources: contracts.StepResourceSpec{
			CPU:    types.CPUmilli(750),        // 0.75 CPU → 750m
			Memory: types.Bytes(1 << 30),       // 1 GiB
			Disk:   types.Bytes(5 * (1 << 30)), // 5 GiB
		},
	}

	spec, err := buildContainerSpec(types.RunID("run-limits-1"), types.JobID("job-limits-1"), manifest, "/tmp/ws", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// CPU millis → NanoCPUs (millis * 1e6)
	wantNano := int64(750 * 1_000_000)
	if spec.LimitNanoCPUs != wantNano {
		t.Fatalf("LimitNanoCPUs=%d, want %d", spec.LimitNanoCPUs, wantNano)
	}
	// Memory bytes preserved.
	if spec.LimitMemoryBytes != int64(1<<30) {
		t.Fatalf("LimitMemoryBytes=%d, want %d", spec.LimitMemoryBytes, int64(1<<30))
	}
	// Disk limit bytes preserved and storage option populated as raw bytes string.
	if spec.LimitDiskBytes != int64(5*(1<<30)) {
		t.Fatalf("LimitDiskBytes=%d, want %d", spec.LimitDiskBytes, int64(5*(1<<30)))
	}
	if spec.StorageSizeOpt != "5368709120" { // 5 GiB in bytes
		t.Fatalf("StorageSizeOpt=%q, want %q", spec.StorageSizeOpt, "5368709120")
	}
}

func TestBuildContainerSpec_ResourceLimitsZeroUnlimited(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    types.StepID("step-unlimited"),
		Name:  "No Limits",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadOnly,
			SnapshotCID: types.CID("bafy123"),
		}},
		// Zero values mean unlimited.
		Resources: contracts.StepResourceSpec{},
	}

	spec, err := buildContainerSpec(types.RunID("run-limits-2"), types.JobID("job-limits-2"), manifest, "/tmp/ws", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	if spec.LimitNanoCPUs != 0 || spec.LimitMemoryBytes != 0 || spec.LimitDiskBytes != 0 {
		t.Fatalf("limits were set when zero/unlimited requested: %+v", spec)
	}
	if spec.StorageSizeOpt != "" {
		t.Fatalf("StorageSizeOpt=%q, want empty", spec.StorageSizeOpt)
	}
}
