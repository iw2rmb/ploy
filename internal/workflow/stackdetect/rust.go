package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/fsutil"
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
	if fsutil.FileExists(cargoPath) {
		content, err := os.ReadFile(cargoPath)
		if err == nil {
			if matches := rustVersionRegex.FindStringSubmatch(string(content)); matches != nil {
				version := canonicalizeRustVersion(matches[1])
				return &Observation{
					Language: "rust",
					Tool:     "cargo",
					Release:  &version,
					Evidence: []EvidenceItem{
						{Path: "Cargo.toml", Key: "rust-version", Value: version},
					},
				}, nil
			}
		}
	}

	// 2. Check rust-toolchain.toml for channel.
	if fsutil.FileExists(toolchainTomlPath) {
		content, err := os.ReadFile(toolchainTomlPath)
		if err == nil {
			if matches := rustToolchainChannelRegex.FindStringSubmatch(string(content)); matches != nil {
				channel := strings.TrimSpace(matches[1])

				// Check for non-deterministic channels.
				if isNonDeterministicChannel(channel) {
					return nil, &DetectionError{
						Reason:  "unknown",
						Message: "rust-toolchain.toml specifies non-deterministic channel: " + channel,
						Evidence: []EvidenceItem{
							{Path: "rust-toolchain.toml", Key: "channel", Value: channel},
						},
					}
				}

				// Check if channel is a numeric version.
				if version := extractNumericRustVersion(channel); version != "" {
					return &Observation{
						Language: "rust",
						Tool:     "cargo",
						Release:  &version,
						Evidence: []EvidenceItem{
							{Path: "rust-toolchain.toml", Key: "channel", Value: version},
						},
					}, nil
				}

				// Non-numeric, non-standard channel.
				return nil, &DetectionError{
					Reason:  "unknown",
					Message: "rust-toolchain.toml specifies non-numeric channel: " + channel,
					Evidence: []EvidenceItem{
						{Path: "rust-toolchain.toml", Key: "channel", Value: channel},
					},
				}
			}
		}
	}

	// 3. Check rust-toolchain plain file.
	if fsutil.FileExists(toolchainPath) {
		content, err := os.ReadFile(toolchainPath)
		if err == nil {
			channel := strings.TrimSpace(string(content))

			// Check for non-deterministic channels.
			if isNonDeterministicChannel(channel) {
				return nil, &DetectionError{
					Reason:  "unknown",
					Message: "rust-toolchain specifies non-deterministic channel: " + channel,
					Evidence: []EvidenceItem{
						{Path: "rust-toolchain", Key: "channel", Value: channel},
					},
				}
			}

			// Check if channel is a numeric version.
			if version := extractNumericRustVersion(channel); version != "" {
				return &Observation{
					Language: "rust",
					Tool:     "cargo",
					Release:  &version,
					Evidence: []EvidenceItem{
						{Path: "rust-toolchain", Key: "channel", Value: version},
					},
				}, nil
			}
		}
	}

	// No version information found.
	return nil, &DetectionError{
		Reason:  "unknown",
		Message: "no rust-version in Cargo.toml and no numeric channel in rust-toolchain",
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
