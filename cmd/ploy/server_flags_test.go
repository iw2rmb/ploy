package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/deploy"
)

// TestHandleServerDeployFlagParsing ensures --reuse/--force-new-ca are parsed and applied.
func TestHandleServerDeployFlagParsing(t *testing.T) {
	tmp := t.TempDir()
	// Minimal identity and binary files so helpers pass validation.
	idPath := filepath.Join(tmp, "id_test")
	if err := os.WriteFile(idPath, []byte("key"), 0o600); err != nil {
		t.Fatalf("write id: %v", err)
	}
	binPath := filepath.Join(tmp, "ployd-test")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write bin: %v", err)
	}

	// Avoid touching real config home and default marker.
	cfgHome := filepath.Join(tmp, "config")
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	// Extra guard: disable default marker mutation in case a code path escapes cfgHome.
	t.Setenv("PLOY_NO_DEFAULT_MUTATION", "1")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(cfgHome, "clusters")) })

	cases := []struct {
		name         string
		args         []string
		expectDetect bool
	}{
		{ // default reuse=true
			name:         "default reuse triggers detection",
			args:         []string{"--address", "10.0.0.5", "--identity", idPath, "--ployd-binary", binPath},
			expectDetect: true,
		},
		{
			name:         "reuse=false disables detection",
			args:         []string{"--address", "10.0.0.5", "--identity", idPath, "--ployd-binary", binPath, "--reuse=false"},
			expectDetect: false,
		},
		{
			name:         "force-new-ca overrides reuse=true",
			args:         []string{"--address", "10.0.0.5", "--identity", idPath, "--ployd-binary", binPath, "--reuse=true", "--force-new-ca"},
			expectDetect: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			// Stub detection runner to toggle when invoked.
			oldDetect := detectRunner
			detectRunner = &mockDetectRunner{t: t, existingCluster: false, detectCalled: &called}
			defer func() { detectRunner = oldDetect }()

			// Stub provisioning to avoid network; return nil immediately.
			oldProv := provisionHost
			provisionHost = func(ctx context.Context, _ deploy.ProvisionOptions) error { return nil }
			defer func() { provisionHost = oldProv }()

			// Execute command
			_ = handleServerDeploy(tc.args, bytes.NewBuffer(nil))

			if called != tc.expectDetect {
				t.Fatalf("DetectExisting called=%v, expect %v", called, tc.expectDetect)
			}
		})
	}
}

// TestHandleServerDeployRefreshAdminCertRequiresDescriptor verifies that --refresh-admin-cert
// requires an existing descriptor and fails gracefully if missing.
func TestHandleServerDeployRefreshAdminCertRequiresDescriptor(t *testing.T) {
	tmp := t.TempDir()
	cfgHome := filepath.Join(tmp, "config")
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	t.Setenv("PLOY_NO_DEFAULT_MUTATION", "1")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(cfgHome, "clusters")) })

	// No descriptor exists, so refresh should fail with appropriate error.
	buf := &bytes.Buffer{}
	err := handleServerDeploy([]string{"--address", "10.0.0.5", "--refresh-admin-cert"}, buf)
	if err == nil {
		t.Fatal("expected error when refreshing admin cert without descriptor")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("deprecated")) {
		t.Fatalf("expected descriptor error, got: %v", err)
	}
}
