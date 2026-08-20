package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Regex patterns for Rust version detection.
var (
	// rustVersionRegex matches rust-version = "1.xx" or rust-version = "1.xx.x" in Cargo.toml.
	rustVersionRegex = regexp.MustCompile(`(?m)^rust-version\s*=\s*"(1\.\d+(?:\.\d+)?)"`)

	// rustToolchainChannelRegex matches channel = "..." in rust-toolchain.toml.
	rustToolchainChannelRegex = regexp.MustCompile(`(?m)^channel\s*=\s*"([^"]+)"`)

	// rustNumericVersionRegex validates and extracts numeric version like "1.75" or "1.75.0".
	rustNumericVersionRegex = regexp.MustCompile(`^(1\.\d+)(?:\.\d+)?$`)
)

// detectRust detects Rust version from Cargo.toml, rust-toolchain.toml, or rust-toolchain.
// Precedence order:
//  1. Cargo.toml rust-version = "1.76" → preferred
//  2. rust-toolchain.toml channel = "1.75" → numeric only
//  3. rust-toolchain plain file → numeric only
//
// Channels like "stable" or "nightly" are non-deterministic and return unknown.
func detectRust(ctx context.Context, workspace string) (*Observation, error) {
	cargoPath := filepath.Join(workspace, "Cargo.toml")
	toolchainTomlPath := filepath.Join(workspace, "rust-toolchain.toml")
	toolchainPath := filepath.Join(workspace, "rust-toolchain")

	// 1. Check Cargo.toml for rust-version (highest precedence).
	if fileExists(cargoPath) {
		content, err := os.ReadFile(cargoPath)
		if err == nil {
			if matches := rustVersionRegex.FindStringSubmatch(string(content)); matches != nil {
				version := canonicalizeRustVersion(matches[1])
				return rustObservation("Cargo.toml", "rust-version", version), nil
			}
		}
	}

	toolchainSources := []struct {
		path             string
		name             string
		channel          func([]byte) string
		reportNonNumeric bool
	}{
		{path: toolchainTomlPath, name: "rust-toolchain.toml", channel: func(content []byte) string {
			matches := rustToolchainChannelRegex.FindStringSubmatch(string(content))
			if matches == nil {
				return ""
			}
			return strings.TrimSpace(matches[1])
		}, reportNonNumeric: true},
		{path: toolchainPath, name: "rust-toolchain", channel: func(content []byte) string {
			return strings.TrimSpace(string(content))
		}},
	}
	for _, source := range toolchainSources {
		if !fileExists(source.path) {
			continue
		}
		content, err := os.ReadFile(source.path)
		if err != nil {
			continue
		}
		channel := source.channel(content)
		if channel == "" {
			continue
		}
		version, channelType := classifyRustToolchainChannel(channel)
		switch channelType {
		case rustChannelNumeric:
			return rustObservation(source.name, "channel", version), nil
		case rustChannelNonDeterministic:
			return nil, rustChannelError(source.name, "non-deterministic", channel)
		case rustChannelNonNumeric:
			if source.reportNonNumeric {
				return nil, rustChannelError(source.name, "non-numeric", channel)
			}
		}
	}

	// No version information found.
	return nil, &DetectionError{
		Reason:  "unknown",
		Message: "no rust-version in Cargo.toml and no numeric channel in rust-toolchain",
	}
}

type rustChannelType uint8

const (
	rustChannelNonNumeric rustChannelType = iota
	rustChannelNumeric
	rustChannelNonDeterministic
)

func classifyRustToolchainChannel(channel string) (string, rustChannelType) {
	if isNonDeterministicChannel(channel) {
		return "", rustChannelNonDeterministic
	}
	if version := extractNumericRustVersion(channel); version != "" {
		return version, rustChannelNumeric
	}
	return "", rustChannelNonNumeric
}

func rustObservation(path, key, version string) *Observation {
	return &Observation{
		Language: "rust",
		Tool:     "cargo",
		Release:  &version,
		Evidence: []EvidenceItem{{Path: path, Key: key, Value: version}},
	}
}

func rustChannelError(path, classification, channel string) error {
	return &DetectionError{
		Reason:   "unknown",
		Message:  path + " specifies " + classification + " channel: " + channel,
		Evidence: []EvidenceItem{{Path: path, Key: "channel", Value: channel}},
	}
}

// isNonDeterministicChannel returns true for channels that are not deterministic
// (stable, nightly, beta).
func isNonDeterministicChannel(channel string) bool {
	lower := strings.ToLower(strings.TrimSpace(channel))
	return lower == "stable" || lower == "nightly" || lower == "beta" ||
		strings.HasPrefix(lower, "nightly-") || strings.HasPrefix(lower, "beta-")
}

// extractNumericRustVersion extracts and canonicalizes a numeric Rust version.
// Returns empty string if not a valid numeric version.
func extractNumericRustVersion(v string) string {
	matches := rustNumericVersionRegex.FindStringSubmatch(strings.TrimSpace(v))
	if matches == nil {
		return ""
	}
	return matches[1] // Returns "1.xx" (canonicalized, without patch)
}

// canonicalizeRustVersion normalizes "1.76.0" to "1.76".
func canonicalizeRustVersion(v string) string {
	return canonicalizeVersion(v, rustNumericVersionRegex, v)
}
