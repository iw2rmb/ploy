package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// uploadFileWithRetryAndVerification uploads a file with enhanced retry logic and integrity verification
func uploadFileWithRetryAndVerification(storeClient *storage.StorageClient, filePath, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Open file for this attempt
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}

		// Attempt upload with verification
		_, uploadErr := storeClient.PutObject(storeClient.GetArtifactsBucket(), storageKey, f, contentType)
		f.Close()

		if uploadErr == nil {
			// Upload successful, now verify integrity
			verifier := storage.NewIntegrityVerifier(storeClient)
			if info, verifyErr := verifier.VerifyUploadedFile(filePath, storageKey); verifyErr != nil {
				fmt.Printf("Upload attempt %d: integrity verification failed: %v\n", attempt, verifyErr)
				// If this is not the last attempt, continue to retry
				if attempt < maxRetries {
					delay := time.Duration(attempt) * baseDelay
					fmt.Printf("Retrying upload after %v...\n", delay)
					time.Sleep(delay)
					continue
				}
				return fmt.Errorf("integrity verification failed after %d attempts: %w", maxRetries, verifyErr)
			} else {
				// Success: upload and verification both passed
				fmt.Printf("File %s uploaded and verified: %s (size: %d bytes)\n",
					filepath.Base(filePath), info.StorageKey, info.UploadedSize)
				return nil
			}
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}

// uploadArtifactBundleWithUnifiedStorage uploads an artifact bundle using unified storage interface
func uploadArtifactBundleWithUnifiedStorage(ctx context.Context, storageInterface storage.Storage, keyPrefix, artifactPath string) error {
	// Upload main artifact file
	if err := uploadFileWithUnifiedStorage(ctx, storageInterface, artifactPath, keyPrefix+filepath.Base(artifactPath), "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to upload artifact: %w", err)
	}

	// Upload signature file if it exists
	sigPath := artifactPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		if err := uploadFileWithUnifiedStorage(ctx, storageInterface, sigPath, keyPrefix+filepath.Base(sigPath), "application/octet-stream"); err != nil {
			fmt.Printf("Warning: Failed to upload signature file: %v\n", err)
		}
	}

	// Upload SBOM file if it exists
	sbomPath := artifactPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		if err := uploadFileWithUnifiedStorage(ctx, storageInterface, sbomPath, keyPrefix+filepath.Base(sbomPath), "application/json"); err != nil {
			fmt.Printf("Warning: Failed to upload SBOM file: %v\n", err)
		}
	}

	fmt.Printf("Artifact bundle uploaded successfully: %s\n", filepath.Base(artifactPath))
	return nil
}

// uploadFileWithUnifiedStorage uploads a file using unified storage interface with retry logic
func uploadFileWithUnifiedStorage(ctx context.Context, storageInterface storage.Storage, filePath, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Open file for this attempt
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}

		// Attempt upload with unified storage interface
		putOpts := []storage.PutOption{
			storage.WithContentType(contentType),
		}

		uploadErr := storageInterface.Put(ctx, storageKey, f, putOpts...)
		f.Close()

		if uploadErr == nil {
			// Upload successful - unified storage interface doesn't need separate verification
			fmt.Printf("File %s uploaded successfully: %s\n", filepath.Base(filePath), storageKey)
			return nil
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}

// uploadBytesWithUnifiedStorage uploads byte data using unified storage interface with retry logic
func uploadBytesWithUnifiedStorage(ctx context.Context, storageInterface storage.Storage, data []byte, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create new reader for this attempt
		reader := bytes.NewReader(data)

		// Attempt upload with unified storage interface
		putOpts := []storage.PutOption{
			storage.WithContentType(contentType),
		}

		uploadErr := storageInterface.Put(ctx, storageKey, reader, putOpts...)

		if uploadErr == nil {
			// Upload successful
			fmt.Printf("Data uploaded successfully: %s (%d bytes)\n", storageKey, len(data))
			return nil
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}

// uploadBytesWithRetryAndVerification uploads byte data with enhanced retry logic and verification
func uploadBytesWithRetryAndVerification(storeClient *storage.StorageClient, data []byte, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create new reader for this attempt
		reader := bytes.NewReader(data)

		// Attempt upload
		result, uploadErr := storeClient.PutObject(storeClient.GetArtifactsBucket(), storageKey, reader, contentType)

		if uploadErr == nil {
			// Upload successful, verify by checking result and optionally retrieving object
			if result != nil && result.Size == int64(len(data)) {
				fmt.Printf("Data uploaded and size verified: %s (%d bytes)\n", storageKey, result.Size)
				return nil
			} else {
				fmt.Printf("Upload attempt %d: size mismatch (expected %d, got %d)\n",
					attempt, len(data), result.Size)
				// If this is not the last attempt, continue to retry
				if attempt < maxRetries {
					delay := time.Duration(attempt) * baseDelay
					fmt.Printf("Retrying upload after %v...\n", delay)
					time.Sleep(delay)
					continue
				}
				return fmt.Errorf("size verification failed after %d attempts", maxRetries)
			}
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}
