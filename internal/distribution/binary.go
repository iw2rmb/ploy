package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// BinaryInfo represents metadata about a controller binary
type BinaryInfo struct {
	Version     string            `json:"version"`
	BuildTime   time.Time         `json:"build_time"`
	GitCommit   string            `json:"git_commit"`
	Platform    string            `json:"platform"`
	Architecture string           `json:"architecture"`
	SHA256Hash  string            `json:"sha256_hash"`
	Size        int64             `json:"size"`
	Path        string            `json:"path"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// BinaryDistributor handles controller binary distribution
type BinaryDistributor struct {
	storage       storage.StorageProvider
	collection    string
	cacheDir      string
	retentionDays int
}

// NewBinaryDistributor creates a new binary distributor
func NewBinaryDistributor(storageProvider storage.StorageProvider, collection string, cacheDir string) *BinaryDistributor {
	return &BinaryDistributor{
		storage:       storageProvider,
		collection:    collection,
		cacheDir:      cacheDir,
		retentionDays: 30, // Keep binaries for 30 days
	}
}

// UploadBinary uploads a controller binary with version metadata
func (bd *BinaryDistributor) UploadBinary(binaryPath string, info BinaryInfo) error {
	// Calculate SHA256 hash
	hash, size, err := bd.calculateHash(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}
	
	info.SHA256Hash = hash
	info.Size = size
	info.BuildTime = time.Now()
	
	// Create storage key
	storageKey := fmt.Sprintf("api-binaries/%s/%s/%s/api", 
		info.Version, info.Platform, info.Architecture)
	
	// Upload binary
	file, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to open binary: %w", err)
	}
	defer file.Close()
	
	_, err = bd.storage.PutObject(bd.collection, storageKey, file, "application/octet-stream")
	if err != nil {
		return fmt.Errorf("failed to upload binary: %w", err)
	}
	
	// Upload metadata
	metadataKey := fmt.Sprintf("api-binaries/%s/%s/%s/metadata.json", 
		info.Version, info.Platform, info.Architecture)
	
	metadataBytes, err := info.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}
	
	metadataReader := &stringReadSeeker{content: string(metadataBytes)}
	_, err = bd.storage.PutObject(bd.collection, metadataKey, metadataReader, "application/json")
	if err != nil {
		return fmt.Errorf("failed to upload metadata: %w", err)
	}
	
	return nil
}

// DownloadBinary downloads and caches a controller binary
func (bd *BinaryDistributor) DownloadBinary(version, platform, architecture string) (string, *BinaryInfo, error) {
	// Check local cache first
	localPath := filepath.Join(bd.cacheDir, version, platform, architecture, "controller")
	if bd.isCachedAndValid(localPath, version, platform, architecture) {
		info, err := bd.getCachedMetadata(version, platform, architecture)
		if err == nil {
			return localPath, info, nil
		}
	}
	
	// Download from storage
	storageKey := fmt.Sprintf("api-binaries/%s/%s/%s/api", 
		version, platform, architecture)
	
	reader, err := bd.storage.GetObject(bd.collection, storageKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to download binary: %w", err)
	}
	defer reader.Close()
	
	// Create cache directory
	cacheDir := filepath.Dir(localPath)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	
	// Write to cache
	outFile, err := os.Create(localPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create cache file: %w", err)
	}
	defer outFile.Close()
	
	if _, err := io.Copy(outFile, reader); err != nil {
		return "", nil, fmt.Errorf("failed to write binary: %w", err)
	}
	
	// Make executable
	if err := os.Chmod(localPath, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to make binary executable: %w", err)
	}
	
	// Download and cache metadata
	info, err := bd.downloadMetadata(version, platform, architecture)
	if err != nil {
		return "", nil, fmt.Errorf("failed to download metadata: %w", err)
	}
	
	// Verify integrity
	if err := bd.verifyBinaryIntegrity(localPath, info); err != nil {
		os.Remove(localPath) // Clean up invalid binary
		return "", nil, fmt.Errorf("binary integrity verification failed: %w", err)
	}
	
	return localPath, info, nil
}

// ListVersions lists available controller binary versions
func (bd *BinaryDistributor) ListVersions() ([]string, error) {
	objects, err := bd.storage.ListObjects(bd.collection, "api-binaries/")
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	
	versions := make(map[string]bool)
	for _, obj := range objects {
		// obj.Key contains the directory name (version) directly
		// from our SeaweedFS listing, e.g., "test", "v1.0.0-test"
		if obj.Key != "" && obj.Key != "." && obj.Key != ".." {
			versions[obj.Key] = true
		}
	}
	
	var result []string
	for version := range versions {
		result = append(result, version)
	}
	
	return result, nil
}

// CleanupOldVersions removes old binary versions from storage
func (bd *BinaryDistributor) CleanupOldVersions() error {
	cutoffTime := time.Now().AddDate(0, 0, -bd.retentionDays)
	
	objects, err := bd.storage.ListObjects(bd.collection, "api-binaries/")
	if err != nil {
		return fmt.Errorf("failed to list objects for cleanup: %w", err)
	}
	
	for _, obj := range objects {
		// Parse last modified time
		lastModified, err := time.Parse(time.RFC3339, obj.LastModified)
		if err != nil {
			continue // Skip if we can't parse time
		}
		
		if lastModified.Before(cutoffTime) {
			// TODO: Implement delete functionality in storage interface
			// For now, just log that we would delete this
			fmt.Printf("Would delete old binary: %s (modified: %s)\n", obj.Key, obj.LastModified)
		}
	}
	
	return nil
}

// calculateHash calculates SHA256 hash and size of a file
func (bd *BinaryDistributor) calculateHash(filePath string) (string, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	
	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash, size, nil
}

// verifyBinaryIntegrity verifies the integrity of a downloaded binary
func (bd *BinaryDistributor) verifyBinaryIntegrity(filePath string, info *BinaryInfo) error {
	hash, size, err := bd.calculateHash(filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate hash for verification: %w", err)
	}
	
	if hash != info.SHA256Hash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", info.SHA256Hash, hash)
	}
	
	if size != info.Size {
		return fmt.Errorf("size mismatch: expected %d, got %d", info.Size, size)
	}
	
	return nil
}

// isCachedAndValid checks if a binary is cached and valid
func (bd *BinaryDistributor) isCachedAndValid(localPath, version, platform, architecture string) bool {
	stat, err := os.Stat(localPath)
	if err != nil {
		return false
	}
	
	// Check if file is too old (more than 1 day)
	if time.Since(stat.ModTime()) > 24*time.Hour {
		return false
	}
	
	// Check if metadata exists
	metadataPath := filepath.Join(bd.cacheDir, version, platform, architecture, "metadata.json")
	if _, err := os.Stat(metadataPath); err != nil {
		return false
	}
	
	return true
}

// getCachedMetadata reads cached metadata
func (bd *BinaryDistributor) getCachedMetadata(version, platform, architecture string) (*BinaryInfo, error) {
	metadataPath := filepath.Join(bd.cacheDir, version, platform, architecture, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}
	
	var info BinaryInfo
	if err := info.FromJSON(data); err != nil {
		return nil, err
	}
	
	return &info, nil
}

// downloadMetadata downloads and caches metadata
func (bd *BinaryDistributor) downloadMetadata(version, platform, architecture string) (*BinaryInfo, error) {
	metadataKey := fmt.Sprintf("api-binaries/%s/%s/%s/metadata.json", 
		version, platform, architecture)
	
	reader, err := bd.storage.GetObject(bd.collection, metadataKey)
	if err != nil {
		return nil, fmt.Errorf("failed to download metadata: %w", err)
	}
	defer reader.Close()
	
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}
	
	var info BinaryInfo
	if err := info.FromJSON(data); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}
	
	// Cache metadata locally
	metadataPath := filepath.Join(bd.cacheDir, version, platform, architecture, "metadata.json")
	cacheDir := filepath.Dir(metadataPath)
	if err := os.MkdirAll(cacheDir, 0755); err == nil {
		os.WriteFile(metadataPath, data, 0644)
	}
	
	return &info, nil
}

// stringReadSeeker implements io.ReadSeeker for strings
type stringReadSeeker struct {
	content string
	pos     int64
}

func (s *stringReadSeeker) Read(p []byte) (int, error) {
	if s.pos >= int64(len(s.content)) {
		return 0, io.EOF
	}
	
	n := copy(p, s.content[s.pos:])
	s.pos += int64(n)
	return n, nil
}

func (s *stringReadSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		s.pos = offset
	case io.SeekCurrent:
		s.pos += offset
	case io.SeekEnd:
		s.pos = int64(len(s.content)) + offset
	}
	
	if s.pos < 0 {
		s.pos = 0
	}
	if s.pos > int64(len(s.content)) {
		s.pos = int64(len(s.content))
	}
	
	return s.pos, nil
}