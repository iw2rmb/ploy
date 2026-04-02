package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveListAndDefaultDescriptor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmp)
	t.Cleanup(func() {
		if dir, err := configBaseDir(); err == nil {
			_ = os.RemoveAll(dir)
		}
	})

	// Saving without cluster id should fail.
	if _, err := SaveDescriptor(Descriptor{}); err == nil {
		t.Fatalf("expected error when saving without cluster id")
	}

	d := Descriptor{ClusterID: ClusterID("cluster-1"), Address: "10.0.0.1:8443", SSHIdentityPath: "/root/.ssh/id_rsa"}
	if _, err := SaveDescriptor(d); err != nil {
		t.Fatalf("SaveDescriptor error: %v", err)
	}

	// Mark default and verify ListDescriptors marks Default=true.
	if err := SetDefault(ClusterID("cluster-1")); err != nil {
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

func TestConfigBaseDirEnvPrecedenceAndSanitize(t *testing.T) {
	// PLOY_CONFIG_HOME wins over home default.
	tmp := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmp)
	dir, err := configBaseDir()
	if err != nil {
		t.Fatalf("configBaseDir error: %v", err)
	}
	if dir != tmp {
		t.Fatalf("configBaseDir=%s want %s", dir, tmp)
	}

	if got := sanitizeFilename("a/b\\c"); got != "a_b_c" {
		t.Fatalf("sanitizeFilename=%q want a_b_c", got)
	}

	// With env empty, ensure it resolves under home.
	t.Setenv("PLOY_CONFIG_HOME", "")
	// Override HOME via os.UserHomeDir by setting HOME for most systems.
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err = configBaseDir()
	if err != nil {
		t.Fatalf("configBaseDir error: %v", err)
	}
	if want := filepath.Join(home, ".config", "ploy"); dir != want {
		t.Fatalf("configBaseDir=%s want %s", dir, want)
	}

	// Ensure SetDefault creates marker file under config base dir.
	if err := SetDefault(ClusterID("abc")); err != nil {
		t.Fatalf("SetDefault error: %v", err)
	}
	base, err := configBaseDir()
	if err != nil {
		t.Fatalf("configBaseDir error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "default")); err != nil {
		t.Fatalf("default marker not created: %v", err)
	}
}

func TestListDescriptorsMissingDirOK(t *testing.T) {
	t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
	// Deliberately do not create cluster descriptor directories; function should not error and return empty list.
	list, err := ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %v", list)
	}
}

func TestLoadDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmp)

	// LoadDefault should fail when no default is set.
	if _, err := LoadDefault(); err == nil {
		t.Fatal("LoadDefault should fail when no default is set")
	}

	// Save a descriptor and set it as default.
	d := Descriptor{
		ClusterID:       ClusterID("test-cluster"),
		Address:         "10.0.0.1:8443",
		SSHIdentityPath: "/root/.ssh/id_rsa",
		Token:           "dummy",
	}
	if _, err := SaveDescriptor(d); err != nil {
		t.Fatalf("SaveDescriptor error: %v", err)
	}
	if err := SetDefault(ClusterID("test-cluster")); err != nil {
		t.Fatalf("SetDefault error: %v", err)
	}

	// LoadDefault should now return the descriptor.
	loaded, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault error: %v", err)
	}
	if loaded.ClusterID != "test-cluster" {
		t.Errorf("expected cluster ID 'test-cluster', got %q", loaded.ClusterID)
	}
	if loaded.Address != "10.0.0.1:8443" {
		t.Errorf("expected address '10.0.0.1:8443', got %q", loaded.Address)
	}
	if !loaded.Default {
		t.Error("expected Default to be true")
	}
}
