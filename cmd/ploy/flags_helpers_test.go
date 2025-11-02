package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFileReadable_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	if err := validateFileReadable(testFile); err != nil {
		t.Fatalf("expected no error for readable file, got: %v", err)
	}
}

func TestValidateFileReadable_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nonexistent.txt")

	err := validateFileReadable(testFile)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "file does not exist") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateFileReadable_IsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}

	err := validateFileReadable(subDir)
	if err == nil {
		t.Fatal("expected error for directory")
	}
	if !strings.Contains(err.Error(), "path is a directory") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateFileReadable_NotReadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unreadable.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0000); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	err := validateFileReadable(testFile)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
	if !strings.Contains(err.Error(), "cannot read file") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateSSHPort_Valid(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"standard SSH", 22},
		{"custom SSH", 2222},
		{"min port", 1},
		{"max port", 65535},
		{"high port", 8443},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateSSHPort(tt.port); err != nil {
				t.Fatalf("expected valid port %d to pass, got: %v", tt.port, err)
			}
		})
	}
}

func TestValidateSSHPort_Invalid(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 65536},
		{"way too high", 99999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSSHPort(tt.port)
			if err == nil {
				t.Fatalf("expected error for invalid port %d", tt.port)
			}
			if !strings.Contains(err.Error(), "invalid SSH port") {
				t.Fatalf("unexpected error message: %v", err)
			}
		})
	}
}

func TestResolveIdentityPath_Explicit(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "my_key")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test key: %v", err)
	}

	resolved, err := resolveIdentityPath(stringValue{set: true, value: keyPath})
	if err != nil {
		t.Fatalf("resolveIdentityPath error: %v", err)
	}
	if resolved != keyPath {
		t.Fatalf("expected %q, got %q", keyPath, resolved)
	}
}

func TestResolveIdentityPath_ExplicitNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "nonexistent_key")

	_, err := resolveIdentityPath(stringValue{set: true, value: keyPath})
	if err == nil {
		t.Fatal("expected error for non-existent identity file")
	}
	if !strings.Contains(err.Error(), "identity file") && !strings.Contains(err.Error(), "file does not exist") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestResolveIdentityPath_TildeExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "my_key")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test key: %v", err)
	}

	// Mock home directory expansion by using relative path handling.
	// Since expandPath is tested separately, we just ensure tilde works.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home directory: %v", err)
	}

	// Create a temporary key in a known location.
	sshDir := filepath.Join(homeDir, ".ssh")
	if _, err := os.Stat(sshDir); os.IsNotExist(err) {
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			t.Skipf("cannot create .ssh directory: %v", err)
		}
		// Cleanup after test.
		defer func() {
			_ = os.RemoveAll(sshDir)
		}()
	}

	testKey := filepath.Join(sshDir, "test_id_rsa")
	if err := os.WriteFile(testKey, []byte("test key"), 0600); err != nil {
		t.Skipf("cannot create test key: %v", err)
	}
	defer func() {
		_ = os.Remove(testKey)
	}()

	resolved, err := resolveIdentityPath(stringValue{set: true, value: "~/.ssh/test_id_rsa"})
	if err != nil {
		t.Fatalf("resolveIdentityPath error: %v", err)
	}
	if resolved != testKey {
		t.Fatalf("expected %q, got %q", testKey, resolved)
	}
}

func TestResolvePloydBinaryPath_Explicit(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}

	resolved, err := resolvePloydBinaryPath(stringValue{set: true, value: binPath})
	if err != nil {
		t.Fatalf("resolvePloydBinaryPath error: %v", err)
	}
	if resolved != binPath {
		t.Fatalf("expected %q, got %q", binPath, resolved)
	}
}

func TestResolvePloydBinaryPath_ExplicitNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "nonexistent_ployd")

	_, err := resolvePloydBinaryPath(stringValue{set: true, value: binPath})
	if err == nil {
		t.Fatal("expected error for non-existent binary")
	}
	if !strings.Contains(err.Error(), "ployd binary") && !strings.Contains(err.Error(), "file does not exist") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestExpandPath_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home directory: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde only", "~", home},
		{"tilde with path", "~/.ssh/id_rsa", filepath.Join(home, ".ssh", "id_rsa")},
		{"absolute path", "/etc/passwd", "/etc/passwd"},
		{"relative path", "foo/bar", "foo/bar"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
