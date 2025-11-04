package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// writeLocalAdminBundle writes CA and admin cert/key under the config home.
func writeLocalAdminBundle(clusterID, caPEM, certPEM, keyPEM string) (caPath, certPath, keyPath string, err error) {
	base, err := resolveConfigBaseDir()
	if err != nil {
		return "", "", "", err
	}
	dir := filepath.Join(base, "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", err
	}
	caPath = filepath.Join(dir, fmt.Sprintf("%s-ca.crt", clusterID))
	certPath = filepath.Join(dir, fmt.Sprintf("%s-admin.crt", clusterID))
	keyPath = filepath.Join(dir, fmt.Sprintf("%s-admin.key", clusterID))
	if err := os.WriteFile(caPath, []byte(strings.TrimSpace(caPEM)+"\n"), 0o644); err != nil {
		return "", "", "", err
	}
	if err := os.WriteFile(certPath, []byte(strings.TrimSpace(certPEM)+"\n"), 0o644); err != nil {
		return "", "", "", err
	}
	// Ensure 0600 for the private key regardless of umask
	if err := writeFile0600(keyPath, []byte(strings.TrimSpace(keyPEM)+"\n")); err != nil {
		return "", "", "", err
	}
	return caPath, certPath, keyPath, nil
}

func writeFile0600(path string, data []byte) error {
	// Atomic write with proper mode
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	// os.Rename preserves mode bits
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Ensure mode is exactly 0600
	return os.Chmod(path, fs.FileMode(0o600))
}

// resolveConfigBaseDir mirrors internal/cli/config clusters dir resolution to find the base.
func resolveConfigBaseDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME"))
	if base == "" {
		xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if xdg != "" {
			base = filepath.Join(xdg, "ploy")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".config", "ploy")
		}
	}
	return base, nil
}
