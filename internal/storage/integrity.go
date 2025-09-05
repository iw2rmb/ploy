package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// IntegrityInfo contains verification data for an uploaded artifact
type IntegrityInfo struct {
	LocalPath        string `json:"local_path"`
	StorageKey       string `json:"storage_key"`
	LocalSize        int64  `json:"local_size"`
	UploadedSize     int64  `json:"uploaded_size"`
	LocalChecksum    string `json:"local_checksum"`
	UploadedHash     string `json:"uploaded_hash"`
	Verified         bool   `json:"verified"`
	VerificationTime string `json:"verification_time"`
}

// IntegrityVerifier provides comprehensive artifact integrity verification
type IntegrityVerifier struct {
	storage StorageProvider
}

// NewIntegrityVerifier creates a new integrity verification instance
func NewIntegrityVerifier(storage StorageProvider) *IntegrityVerifier {
	return &IntegrityVerifier{
		storage: storage,
	}
}

// VerifyArtifactBundle performs comprehensive integrity verification of uploaded artifact bundle
func (v *IntegrityVerifier) VerifyArtifactBundle(keyPrefix, localPath string) (*BundleIntegrityResult, error) {
	result := &BundleIntegrityResult{
		MainArtifact: &IntegrityInfo{LocalPath: localPath},
		KeyPrefix:    keyPrefix,
		Verified:     true,
	}

	// Verify main artifact
	if err := v.verifyFile(localPath, keyPrefix+filepath.Base(localPath), result.MainArtifact); err != nil {
		result.Verified = false
		result.Errors = append(result.Errors, fmt.Sprintf("main artifact verification failed: %v", err))
	}

	// Verify SBOM if exists
	sbomPath := localPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		result.SBOM = &IntegrityInfo{LocalPath: sbomPath}
		if err := v.verifyFile(sbomPath, keyPrefix+filepath.Base(sbomPath), result.SBOM); err != nil {
			result.Verified = false
			result.Errors = append(result.Errors, fmt.Sprintf("SBOM verification failed: %v", err))
		} else {
			// Additional SBOM content validation
			if err := v.validateSBOMContent(sbomPath); err != nil {
				result.Verified = false
				result.Errors = append(result.Errors, fmt.Sprintf("SBOM content validation failed: %v", err))
			}
		}
	}

	// Verify signature if exists
	sigPath := localPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		result.Signature = &IntegrityInfo{LocalPath: sigPath}
		if err := v.verifyFile(sigPath, keyPrefix+filepath.Base(sigPath), result.Signature); err != nil {
			result.Verified = false
			result.Errors = append(result.Errors, fmt.Sprintf("signature verification failed: %v", err))
		}
	}

	// Verify certificate if exists
	crtPath := localPath + ".crt"
	if _, err := os.Stat(crtPath); err == nil {
		result.Certificate = &IntegrityInfo{LocalPath: crtPath}
		if err := v.verifyFile(crtPath, keyPrefix+filepath.Base(crtPath), result.Certificate); err != nil {
			result.Verified = false
			result.Errors = append(result.Errors, fmt.Sprintf("certificate verification failed: %v", err))
		}
	}

	return result, nil
}

// VerifyUploadedFile performs integrity verification for a single uploaded file
func (v *IntegrityVerifier) VerifyUploadedFile(localPath, storageKey string) (*IntegrityInfo, error) {
	info := &IntegrityInfo{
		LocalPath:  localPath,
		StorageKey: storageKey,
	}

	return info, v.verifyFile(localPath, storageKey, info)
}

// verifyFile performs comprehensive verification of a single file
func (v *IntegrityVerifier) verifyFile(localPath, storageKey string, info *IntegrityInfo) error {
	// Get local file info
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	localStat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}
	info.LocalSize = localStat.Size()

	// Calculate local checksum
	localChecksum, err := v.calculateChecksum(localFile)
	if err != nil {
		return fmt.Errorf("failed to calculate local checksum: %w", err)
	}
	info.LocalChecksum = localChecksum

	// Verify file exists in storage
	if err := v.storage.VerifyUpload(storageKey); err != nil {
		return fmt.Errorf("file not found in storage: %w", err)
	}

	// Download and verify uploaded file
	uploadedReader, err := v.storage.GetObject(v.storage.GetArtifactsBucket(), storageKey)
	if err != nil {
		return fmt.Errorf("failed to download uploaded file: %w", err)
	}
	defer uploadedReader.Close()

	// Count bytes and calculate checksum of uploaded file
	uploadedChecksum, uploadedSize, err := v.calculateChecksumAndSize(uploadedReader)
	if err != nil {
		return fmt.Errorf("failed to verify uploaded file: %w", err)
	}

	info.UploadedSize = uploadedSize
	info.UploadedHash = uploadedChecksum

	// Verify size matches
	if info.LocalSize != info.UploadedSize {
		return fmt.Errorf("size mismatch: local=%d bytes, uploaded=%d bytes", info.LocalSize, info.UploadedSize)
	}

	// Verify checksum matches
	if info.LocalChecksum != info.UploadedHash {
		return fmt.Errorf("checksum mismatch: local=%s, uploaded=%s", info.LocalChecksum, info.UploadedHash)
	}

	info.Verified = true
	return nil
}

// calculateChecksum computes SHA-256 checksum of a file
func (v *IntegrityVerifier) calculateChecksum(reader io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// calculateChecksumAndSize computes both SHA-256 checksum and size
func (v *IntegrityVerifier) calculateChecksumAndSize(reader io.Reader) (string, int64, error) {
	hasher := sha256.New()
	size, err := io.Copy(hasher, reader)
	if err != nil {
		return "", 0, err
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	return checksum, size, nil
}

// validateSBOMContent performs basic SBOM content validation
func (v *IntegrityVerifier) validateSBOMContent(sbomPath string) error {
	file, err := os.Open(sbomPath)
	if err != nil {
		return fmt.Errorf("failed to open SBOM file: %w", err)
	}
	defer file.Close()

	// Parse as JSON to validate basic structure
	var sbomData map[string]interface{}
	if err := json.NewDecoder(file).Decode(&sbomData); err != nil {
		return fmt.Errorf("SBOM is not valid JSON: %w", err)
	}

	// Check for required SPDX fields
	requiredFields := []string{"spdxVersion", "dataLicense", "SPDXID", "name", "documentNamespace"}
	for _, field := range requiredFields {
		if _, exists := sbomData[field]; !exists {
			return fmt.Errorf("SBOM missing required field: %s", field)
		}
	}

	// Validate SPDX version format
	if spdxVersion, ok := sbomData["spdxVersion"].(string); ok {
		if !strings.HasPrefix(spdxVersion, "SPDX-") {
			return fmt.Errorf("invalid SPDX version format: %s", spdxVersion)
		}
	}

	return nil
}

// BundleIntegrityResult represents the result of bundle integrity verification
type BundleIntegrityResult struct {
	KeyPrefix    string         `json:"key_prefix"`
	MainArtifact *IntegrityInfo `json:"main_artifact"`
	SBOM         *IntegrityInfo `json:"sbom,omitempty"`
	Signature    *IntegrityInfo `json:"signature,omitempty"`
	Certificate  *IntegrityInfo `json:"certificate,omitempty"`
	Verified     bool           `json:"verified"`
	Errors       []string       `json:"errors,omitempty"`
}

// GetVerificationSummary returns a human-readable summary of verification results
func (r *BundleIntegrityResult) GetVerificationSummary() string {
	if r.Verified {
		fileCount := 1 // main artifact
		if r.SBOM != nil && r.SBOM.Verified {
			fileCount++
		}
		if r.Signature != nil && r.Signature.Verified {
			fileCount++
		}
		if r.Certificate != nil && r.Certificate.Verified {
			fileCount++
		}
		return fmt.Sprintf("✓ Bundle integrity verified: %d files validated successfully", fileCount)
	}

	return fmt.Sprintf("✗ Bundle integrity verification failed: %s", strings.Join(r.Errors, "; "))
}
