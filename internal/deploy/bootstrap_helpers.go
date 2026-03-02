package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// This file contains bootstrap helper utilities shared across provisioning flows.

// BuildSSHArgs constructs SSH args with non-interactive defaults suitable for bootstrap.
func BuildSSHArgs(identity string, port int) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if trimmed := strings.TrimSpace(identity); trimmed != "" {
		args = append(args, "-i", trimmed)
	}
	if port != DefaultSSHPort {
		args = append(args, "-p", strconv.Itoa(port))
	}
	return args
}

// BuildScpArgs constructs scp args with strict host key defaults.
func BuildScpArgs(identity string, port int) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if trimmed := strings.TrimSpace(identity); trimmed != "" {
		args = append(args, "-i", trimmed)
	}
	if port != DefaultSSHPort {
		args = append(args, "-P", strconv.Itoa(port))
	}
	return args
}

// RandomHexString returns a random hex string of the requested length.
func RandomHexString(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("bootstrap: random length must be positive")
	}
	buf := make([]byte, (length+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("bootstrap: random token: %w", err)
	}
	hexStr := hex.EncodeToString(buf)
	if len(hexStr) > length {
		hexStr = hexStr[:length]
	}
	return hexStr, nil
}

// GenerateClusterID creates a new cluster identifier using random bytes.
// Returns a string in the format "cluster-<16 hex chars>".
func GenerateClusterID() (string, error) {
	hexPart, err := RandomHexString(16)
	if err != nil {
		return "", fmt.Errorf("generate cluster id: %w", err)
	}
	return fmt.Sprintf("cluster-%s", hexPart), nil
}

// GenerateNodeID creates a new node identifier using NanoID(6).
func GenerateNodeID() string {
	return types.NewNodeKey()
}
