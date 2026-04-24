package step

import (
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildContainerSpec_CertMountOptions(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "ca.crt")
	clientCertPath := filepath.Join(tmpDir, "client.crt")
	clientKeyPath := filepath.Join(tmpDir, "client.key")
	for _, p := range []string{caPath, clientCertPath, clientKeyPath} {
		if err := os.WriteFile(p, []byte("cert"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("step-cert-mounts"),
		Name:  "With Cert Mounts",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
		Options: map[string]any{
			"ploy_ca_cert_path":     caPath,
			"ploy_client_cert_path": clientCertPath,
			"ploy_client_key_path":  clientKeyPath,
		},
	}

	spec, err := buildContainerSpec(types.RunID("run-certs"), types.JobID("job-certs"), manifest, "/tmp/ws", "", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	if len(spec.Mounts) != 4 {
		t.Fatalf("got %d mounts, want 4: %+v", len(spec.Mounts), spec.Mounts)
	}

	expectMount := func(source, target string, readOnly bool) {
		t.Helper()
		for _, m := range spec.Mounts {
			if m.Source == source && m.Target == target {
				if m.ReadOnly != readOnly {
					t.Fatalf("mount %s -> %s ReadOnly=%v, want %v", source, target, m.ReadOnly, readOnly)
				}
				return
			}
		}
		t.Fatalf("mount %s -> %s not found in %+v", source, target, spec.Mounts)
	}

	expectMount(caPath, "/etc/ploy/certs/ca.crt", true)
	expectMount(clientCertPath, "/etc/ploy/certs/client.crt", true)
	expectMount(clientKeyPath, "/etc/ploy/certs/client.key", true)
}

func TestBuildContainerSpec_CertMountOptionsSkipEmptyOrMissing(t *testing.T) {
	manifest := contracts.StepManifest{
		ID:    types.StepID("step-cert-mounts-skip"),
		Name:  "Skip Cert Mounts",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
		Options: map[string]any{
			"ploy_ca_cert_path":     "",
			"ploy_client_cert_path": filepath.Join(t.TempDir(), "missing.crt"),
			"ploy_client_key_path":  filepath.Join(t.TempDir(), "missing.key"),
		},
	}

	spec, err := buildContainerSpec(types.RunID("run-certs-skip"), types.JobID("job-certs-skip"), manifest, "/tmp/ws", "", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	if len(spec.Mounts) != 1 {
		t.Fatalf("got %d mounts, want 1 (workspace only): %+v", len(spec.Mounts), spec.Mounts)
	}
}
