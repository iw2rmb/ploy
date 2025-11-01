package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveListAndDefaultDescriptor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")

	// Saving without cluster id should fail.
	if _, err := SaveDescriptor(Descriptor{}); err == nil {
		t.Fatalf("expected error when saving without cluster id")
	}

	d := Descriptor{ClusterID: "cluster-1", Address: "10.0.0.1:8443", SSHIdentityPath: "/root/.ssh/id_rsa"}
	if _, err := SaveDescriptor(d); err != nil {
		t.Fatalf("SaveDescriptor error: %v", err)
	}

	// Mark default and verify ListDescriptors marks Default=true.
	if err := SetDefault("cluster-1"); err != nil {
		t.Fatalf("SetDefault error: %v", err)
	}
	list, err := ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors error: %v", err)
	}
	if len(list) != 1 || !list[0].Default || list[0].Address != "10.0.0.1:8443" {
		t.Fatalf("unexpected descriptors: %+v", list)
	}
}

func TestClustersDirEnvPrecedenceAndSanitize(t *testing.T) {
	// PLOY_CONFIG_HOME wins over XDG and home.
	tmp := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg"))
	dir, err := clustersDir()
	if err != nil {
		t.Fatalf("clustersDir error: %v", err)
	}
	if !strings.HasPrefix(dir, filepath.Join(tmp, "clusters")) {
		t.Fatalf("clustersDir should be under %s, got %s", filepath.Join(tmp, "clusters"), dir)
	}

	if got := sanitizeFilename("a/b\\c"); got != "a_b_c" {
		t.Fatalf("sanitizeFilename=%q want a_b_c", got)
	}

	// When PLOY_CONFIG_HOME empty, XDG_CONFIG_HOME should be used.
	t.Setenv("PLOY_CONFIG_HOME", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir, err = clustersDir()
	if err != nil {
		t.Fatalf("clustersDir error: %v", err)
	}
	if !strings.HasPrefix(dir, filepath.Join(xdg, "ploy", "clusters")) {
		t.Fatalf("clustersDir should be under %s, got %s", filepath.Join(xdg, "ploy", "clusters"), dir)
	}

	// Finally, with both envs empty, ensure it resolves under home.
	t.Setenv("PLOY_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	// Override HOME via os.UserHomeDir by setting HOME for most systems.
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err = clustersDir()
	if err != nil {
		t.Fatalf("clustersDir error: %v", err)
	}
	if want := filepath.Join(home, ".config", "ploy", "clusters"); dir != want {
		t.Fatalf("clustersDir=%s want %s", dir, want)
	}

	// Ensure SetDefault creates marker file under clusters dir.
	if err := SetDefault("abc"); err != nil {
		t.Fatalf("SetDefault error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "default")); err != nil {
		t.Fatalf("default marker not created: %v", err)
	}
}

func TestListDescriptorsMissingDirOK(t *testing.T) {
	t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	// Deliberately do not create clusters dir; function should not error and return empty list.
	list, err := ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %v", list)
	}
}
