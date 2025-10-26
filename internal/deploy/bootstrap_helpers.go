package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// This file contains bootstrap helper utilities shared across provisioning flows.

// buildSSHArgs constructs SSH args with non-interactive defaults suitable for bootstrap.
func buildSSHArgs(identity string, port int) []string {
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

// buildScpArgs constructs scp args with strict host key defaults.
func buildScpArgs(identity string, port int) []string {
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

// randomHexString returns a random hex string of the requested length.
func randomHexString(length int) (string, error) {
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
