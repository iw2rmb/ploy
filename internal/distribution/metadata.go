package distribution

import (
	"encoding/json"
	"runtime"
	"time"
)

// ToJSON serializes BinaryInfo to JSON bytes
func (bi *BinaryInfo) ToJSON() ([]byte, error) {
	return json.MarshalIndent(bi, "", "  ")
}

// FromJSON deserializes BinaryInfo from JSON bytes
func (bi *BinaryInfo) FromJSON(data []byte) error {
	return json.Unmarshal(data, bi)
}

// GetCurrentPlatformInfo returns platform information for current system
func GetCurrentPlatformInfo() (platform, architecture string) {
	return runtime.GOOS, runtime.GOARCH
}

// CreateBinaryInfo creates BinaryInfo for a binary file
func CreateBinaryInfo(version, gitCommit string, metadata map[string]string) BinaryInfo {
	platform, arch := GetCurrentPlatformInfo()

	if metadata == nil {
		metadata = make(map[string]string)
	}

	// Add build metadata
	metadata["go_version"] = runtime.Version()
	metadata["build_host"] = getHostname()
	metadata["build_user"] = getUsername()

	return BinaryInfo{
		Version:      version,
		BuildTime:    time.Now(),
		GitCommit:    gitCommit,
		Platform:     platform,
		Architecture: arch,
		Metadata:     metadata,
	}
}

// IsCompatibleWith checks if this binary is compatible with target platform
func (bi *BinaryInfo) IsCompatibleWith(targetPlatform, targetArch string) bool {
	return bi.Platform == targetPlatform && bi.Architecture == targetArch
}

// IsNewer checks if this binary is newer than the other
func (bi *BinaryInfo) IsNewer(other *BinaryInfo) bool {
	return bi.BuildTime.After(other.BuildTime)
}

// GetStorageKey returns the storage key for this binary
func (bi *BinaryInfo) GetStorageKey() string {
	return "api-binaries/" + bi.Version + "/" + bi.Platform + "/" + bi.Architecture + "/api"
}

// GetMetadataKey returns the metadata storage key for this binary
func (bi *BinaryInfo) GetMetadataKey() string {
	return "api-binaries/" + bi.Version + "/" + bi.Platform + "/" + bi.Architecture + "/metadata.json"
}

// Validate checks if the binary info is valid
func (bi *BinaryInfo) Validate() error {
	if bi.Version == "" {
		return ErrInvalidVersion
	}
	if bi.Platform == "" {
		return ErrInvalidPlatform
	}
	if bi.Architecture == "" {
		return ErrInvalidArchitecture
	}
	if bi.SHA256Hash == "" {
		return ErrMissingHash
	}
	if bi.Size <= 0 {
		return ErrInvalidSize
	}
	return nil
}

func getHostname() string {
	hostname, err := getSystemHostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func getUsername() string {
	username, err := getSystemUsername()
	if err != nil {
		return "unknown"
	}
	return username
}
