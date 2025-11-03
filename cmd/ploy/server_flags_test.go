package main

import (
	"bytes"
	"context"
	"errors"
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

	// Avoid touching real config home.
	t.Setenv("PLOY_CONFIG_HOME", filepath.Join(tmp, "config"))

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

// The above attempt to directly stub provisionHost with a mismatched type is
// not viable across packages in this isolated test file. Instead, add a small
// shim type matching the signature we need so we can assign a lambda, while we
// still test the flag parsing path by calling handleServerDeploy with a writer.

// deployProvisionOptionsShim mirrors the second parameter type for provisionHost
// but is only used to satisfy the compiler in this test file. The real test does
// not call provisionHost because we intercept before needing it.
type deployProvisionOptionsShim = struct{}

// TestHandleServerDeployRefreshAdminCertWarns verifies UX message when the flag is set.
func TestHandleServerDeployRefreshAdminCertWarns(t *testing.T) {
	tmp := t.TempDir()
	idPath := filepath.Join(tmp, "id_test")
	if err := os.WriteFile(idPath, []byte("key"), 0o600); err != nil {
		t.Fatalf("write id: %v", err)
	}
	binPath := filepath.Join(tmp, "ployd-test")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write bin: %v", err)
	}
	t.Setenv("PLOY_CONFIG_HOME", filepath.Join(tmp, "config"))

	// Ensure detect doesn't run to keep test quick.
	oldDetect := detectRunner
	detectRunner = &mockDetectRunner{t: t, existingCluster: false, detectCalled: new(bool)}
	defer func() { detectRunner = oldDetect }()

	// Stub provisioning to fail fast before any remote work.
	oldProv := provisionHost
	provisionHost = func(_ context.Context, _ deploy.ProvisionOptions) error { return errors.New("stub") }
	defer func() { provisionHost = oldProv }()

	buf := &bytes.Buffer{}
	_ = handleServerDeploy([]string{"--address", "10.0.0.5", "--identity", idPath, "--ployd-binary", binPath, "--refresh-admin-cert"}, buf)
	if !bytes.Contains(buf.Bytes(), []byte("not implemented")) {
		t.Fatalf("expected not-implemented warning in stderr; got: %s", buf.String())
	}
}
