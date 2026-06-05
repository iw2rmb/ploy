package step

import (
	"os"
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

	spec, err := buildContainerSpec(types.RunID("run-in"), types.JobID("job-in"), manifest, "/tmp/ws", "", "/tmp/in", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Expect two mounts: workspace RW and /in.
	if len(spec.Mounts) != 2 {
		t.Fatalf("got %d mounts, want 2: %+v", len(spec.Mounts), spec.Mounts)
	}
	requireMount(t, spec.Mounts, "/in", "/tmp/in", false)
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

	spec, err := buildContainerSpec(types.RunID("run-in"), types.JobID("job-in"), manifest, "/tmp/ws", "", "/tmp/in", "", "", "/tmp/staging")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	requireMount(t, spec.Mounts, "/in", "/tmp/in", false)
	requireNoMount(t, spec.Mounts, "/in/amata.yaml")
}

func TestBuildContainerSpec_ShareMountPresent(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    types.StepID("step-share-mount"),
		Name:  "With /share",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}

	spec, err := buildContainerSpec(types.RunID("run-share"), types.JobID("job-share"), manifest, "/tmp/ws", "", "", "/tmp/share", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	requireMount(t, spec.Mounts, "/share", "/tmp/share", false)
}

func TestBuildContainerSpec_DockerSocketMountOption(t *testing.T) {
	const sock = "/var/run/docker.sock"
	if fi, err := os.Stat(sock); err != nil || fi.IsDir() {
		t.Skipf("%s is not available on this host", sock)
	}

	manifest := contracts.StepManifest{
		ID:      types.StepID("step-docker-socket"),
		Name:    "With Docker socket",
		Image:   "alpine:3",
		Options: map[string]any{"mount_docker_socket": true},
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}

	spec, err := buildContainerSpec(types.RunID("run-docker"), types.JobID("job-docker"), manifest, "/tmp/ws", "", "", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	requireMount(t, spec.Mounts, sock, sock, false)
}
