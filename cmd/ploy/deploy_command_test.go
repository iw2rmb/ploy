package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
	"github.com/iw2rmb/ploy/internal/deploy"
)

func ploydFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ployd")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write temp ployd binary: %v", err)
	}
	return path
}

const (
	adminBootstrapKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ076bootTestAdmin deploy-admin"
	userBootstrapKey  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILoopbackUserKey deploy-user"
)

func authorizedKeysFixture(t *testing.T, keys ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authorized_keys")
	content := strings.Join(keys, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write authorized keys fixture: %v", err)
	}
	return path
}

func TestHandleDeployBootstrapAllowsMissingClusterID(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	var captured deploy.Options
	deployBootstrapRunner = func(_ context.Context, opts deploy.Options) error {
		captured = opts
		return nil
	}

	ploydPath := ploydFixture(t)
	adminKeysPath := authorizedKeysFixture(t, adminBootstrapKey)
	userKeysPath := authorizedKeysFixture(t, userBootstrapKey)
	err := handleDeployBootstrap([]string{
		"--address", "192.0.2.10",
		"--ployd-binary", ploydPath,
		"--admin-authorized-keys", adminKeysPath,
		"--user-authorized-keys", userKeysPath,
	}, io.Discard)
	if err != nil {
		t.Fatalf("expected cluster id to be generated, got error: %v", err)
	}
	if captured.ClusterID == "" {
		t.Fatalf("expected cluster id to be generated")
	}
	if len(captured.InitialBeacons) != 1 {
		t.Fatalf("expected one beacon id, got %v", captured.InitialBeacons)
	}
	if captured.APIKey == "" {
		t.Fatalf("expected api key to be generated")
	}
	if len(captured.InitialWorkers) != 1 || captured.InitialWorkers[0] != captured.InitialBeacons[0] {
		t.Fatalf("expected worker ids to mirror beacon ids, got %v", captured.InitialWorkers)
	}
	beaconID := captured.InitialBeacons[0]
	if !isLowerHexWithLen(beaconID, deploycli.DefaultWorkerIDLength) {
		t.Fatalf("expected beacon id to be lowercase hex length %d, got %q", deploycli.DefaultWorkerIDLength, beaconID)
	}
	if captured.BeaconURL != fmt.Sprintf("https://%s.%s%s", beaconID, captured.ClusterID, deploycli.DefaultDomainSuffix) {
		t.Fatalf("expected beacon url to reference node domain, got %q", captured.BeaconURL)
	}
	if got, want := captured.EtcdEndpoints, []string{"http://192.0.2.10:2379"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected etcd endpoint %v, got %v", want, got)
	}
	if captured.PloydBinaryPath != ploydPath {
		t.Fatalf("expected ployd binary path %q, got %q", ploydPath, captured.PloydBinaryPath)
	}
	if got := captured.AdminAuthorizedKeys; len(got) != 1 || got[0] != adminBootstrapKey {
		t.Fatalf("expected admin authorized key propagated, got %v", got)
	}
	if got := captured.UserAuthorizedKeys; len(got) != 1 || got[0] != userBootstrapKey {
		t.Fatalf("expected user authorized key propagated, got %v", got)
	}
}

func TestHandleDeployBootstrapParsesFlags(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	var captured deploy.Options
	deployBootstrapRunner = func(_ context.Context, opts deploy.Options) error {
		captured = opts
		return nil
	}

	stderr := &bytes.Buffer{}
	ploydPath := ploydFixture(t)
	adminKeysPath := authorizedKeysFixture(t, adminBootstrapKey)
	userKeysPath := authorizedKeysFixture(t, userBootstrapKey)
	err := handleDeployBootstrap([]string{
		"--address", "bootstrap.example.com",
		"--beacon-url", "https://override.example.com",
		"--control-plane-url", "https://control.example.com",
		"--ployd-binary", ploydPath,
		"--admin-authorized-keys", adminKeysPath,
		"--user-authorized-keys", userKeysPath,
	}, stderr)
	if err != nil {
		t.Fatalf("handleDeployBootstrap returned error: %v", err)
	}

	if captured.ClusterID == "" {
		t.Fatalf("expected cluster id to be generated")
	}
	if captured.BeaconURL != "https://override.example.com" {
		t.Fatalf("expected beacon url to propagate")
	}
	if captured.ControlPlaneURL != "https://control.example.com" {
		t.Fatalf("expected control plane url to propagate")
	}
	if got, want := captured.EtcdEndpoints, []string{"http://bootstrap.example.com:2379"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected etcd endpoints %v, got %v", want, got)
	}
	if len(captured.InitialBeacons) != 1 {
		t.Fatalf("expected exactly one beacon id, got %v", captured.InitialBeacons)
	}
	if !isLowerHexWithLen(captured.InitialBeacons[0], deploycli.DefaultWorkerIDLength) {
		t.Fatalf("expected beacon id to be lowercase hex length %d, got %q", deploycli.DefaultWorkerIDLength, captured.InitialBeacons[0])
	}
	if len(captured.InitialWorkers) != 1 || captured.InitialWorkers[0] != captured.InitialBeacons[0] {
		t.Fatalf("expected worker ids to mirror beacon ids, got %v", captured.InitialWorkers)
	}
	if captured.APIKey == "" {
		t.Fatalf("expected api key to be generated")
	}
	if captured.PloydBinaryPath != ploydPath {
		t.Fatalf("expected ployd binary path %q, got %q", ploydPath, captured.PloydBinaryPath)
	}
	if len(captured.AdminAuthorizedKeys) != 1 || captured.AdminAuthorizedKeys[0] != adminBootstrapKey {
		t.Fatalf("expected admin authorized key propagated")
	}
	if len(captured.UserAuthorizedKeys) != 1 || captured.UserAuthorizedKeys[0] != userBootstrapKey {
		t.Fatalf("expected user authorized key propagated")
	}
}

func TestHandleDeployBootstrapGeneratesDefaults(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	var captured deploy.Options
	deployBootstrapRunner = func(_ context.Context, opts deploy.Options) error {
		captured = opts
		return nil
	}

	ploydPath := ploydFixture(t)
	adminKeysPath := authorizedKeysFixture(t, adminBootstrapKey)
	userKeysPath := authorizedKeysFixture(t, userBootstrapKey)
	err := handleDeployBootstrap([]string{
		"--ployd-binary", ploydPath,
		"--admin-authorized-keys", adminKeysPath,
		"--user-authorized-keys", userKeysPath,
	}, io.Discard)
	if err != nil {
		t.Fatalf("handleDeployBootstrap returned error: %v", err)
	}

	if captured.ClusterID == "" {
		t.Fatalf("expected cluster id to be generated")
	}
	if len(captured.ClusterID) != deploycli.DefaultClusterIDLength {
		t.Fatalf("expected cluster id length %d, got %d", deploycli.DefaultClusterIDLength, len(captured.ClusterID))
	}
	if !isLowerHex(captured.ClusterID) {
		t.Fatalf("expected cluster id to be lowercase hex, got %q", captured.ClusterID)
	}
	expectedHost := captured.ClusterID + deploycli.DefaultDomainSuffix
	if captured.Host != expectedHost {
		t.Fatalf("expected host %q, got %q", expectedHost, captured.Host)
	}
	if len(captured.InitialBeacons) != 1 {
		t.Fatalf("expected one beacon id, got %v", captured.InitialBeacons)
	}
	beaconID := captured.InitialBeacons[0]
	if !isLowerHexWithLen(beaconID, deploycli.DefaultWorkerIDLength) {
		t.Fatalf("expected beacon id to be lowercase hex length %d, got %q", deploycli.DefaultWorkerIDLength, beaconID)
	}
	if captured.BeaconURL != fmt.Sprintf("https://%s.%s%s", beaconID, captured.ClusterID, deploycli.DefaultDomainSuffix) {
		t.Fatalf("expected beacon url to default to node domain, got %q", captured.BeaconURL)
	}
	if got, want := captured.InitialWorkers, captured.InitialBeacons; len(got) != len(want) {
		t.Fatalf("expected worker ids to mirror beacon ids, got %v want %v", got, want)
	} else if len(got) > 0 && got[0] != want[0] {
		t.Fatalf("expected worker ids to mirror beacon ids, got %v want %v", got, want)
	}
	expectedEndpoint := fmt.Sprintf("http://%s:2379", expectedHost)
	if got, want := captured.EtcdEndpoints, []string{expectedEndpoint}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected etcd endpoints %v, got %v", want, got)
	}
	if captured.PloydBinaryPath != ploydPath {
		t.Fatalf("expected ployd binary path %q, got %q", ploydPath, captured.PloydBinaryPath)
	}
}
func isLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func isLowerHexWithLen(value string, length int) bool {
	return len(value) == length && isLowerHex(value)
}
