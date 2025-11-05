package config

import (
	"fmt"
	"os"
	"strings"
)

// loadTokenFromFile reads the token from the specified file path.
// The file should contain only the token (whitespace is trimmed).
// Returns an error if the file cannot be read or has incorrect permissions.
func loadTokenFromFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat token file: %w", err)
	}

	// Check file permissions (should be 0600 or more restrictive)
	mode := info.Mode()
	if mode.Perm()&0077 != 0 {
		return "", fmt.Errorf("token file %s has insecure permissions %04o (expected 0600)", path, mode.Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}

	return token, nil
}
