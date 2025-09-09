package arf

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// uploadToStorage uploads a file to the storage backend with retry logic
func (d *OpenRewriteDispatcher) uploadToStorage(ctx context.Context, filePath, storageKey string) error {
	log.Printf("[OpenRewrite Dispatcher] ===== STORAGE UPLOAD START =====")
	log.Printf("[OpenRewrite Dispatcher] Local file path: %s", filePath)
	log.Printf("[OpenRewrite Dispatcher] Storage key: %s", storageKey)
	log.Printf("[OpenRewrite Dispatcher] SeaweedFS URL: %s", d.seaweedfsURL)

	// Check if file exists and get its size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot stat file %s: %v", filePath, err)
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	log.Printf("[OpenRewrite Dispatcher] File exists: size=%d bytes, mode=%v", fileInfo.Size(), fileInfo.Mode())

	// Verify file is readable and not empty
	if fileInfo.Size() == 0 {
		log.Printf("[OpenRewrite Dispatcher] ERROR: File is empty: %s", filePath)
		return fmt.Errorf("file is empty: %s", filePath)
	}

	// Open file for reading
	log.Printf("[OpenRewrite Dispatcher] Opening file for reading...")
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot open file %s: %v", filePath, err)
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()
	log.Printf("[OpenRewrite Dispatcher] File opened successfully")

	// Read entire file into memory (we know the size)
	log.Printf("[OpenRewrite Dispatcher] Reading file into memory (%d bytes)...", fileInfo.Size())
	data := make([]byte, fileInfo.Size())
	startRead := time.Now()
	n, err := io.ReadFull(file, data)
	readDuration := time.Since(startRead)
	if err != nil && err != io.EOF {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to read file %s after %v: %v", filePath, readDuration, err)
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	if int64(n) != fileInfo.Size() {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Size mismatch - read %d bytes but expected %d from %s", n, fileInfo.Size(), filePath)
		return fmt.Errorf("read %d bytes but expected %d from %s", n, fileInfo.Size(), filePath)
	}
	log.Printf("[OpenRewrite Dispatcher] File read successfully: %d bytes in %v", n, readDuration)

	// Log storage client details
	log.Printf("[OpenRewrite Dispatcher] Storage client type: %T", d.storageClient)

	// Upload to storage with retry logic and detailed error tracking
	log.Printf("[OpenRewrite Dispatcher] Starting upload with retry logic...")
	var lastErr error
	for i := 0; i < 3; i++ {
		log.Printf("[OpenRewrite Dispatcher] Upload attempt %d/3 for key: %s", i+1, storageKey)
		attemptStart := time.Now()

		if err := d.storageClient.Put(ctx, storageKey, data); err != nil {
			attemptDuration := time.Since(attemptStart)
			lastErr = err
			log.Printf("[OpenRewrite Dispatcher] Upload attempt %d FAILED after %v: %v", i+1, attemptDuration, err)
			log.Printf("[OpenRewrite Dispatcher] Error type: %T", err)

			if i < 2 { // Don't sleep on the last attempt
				sleepDuration := time.Second * time.Duration(i+1)
				log.Printf("[OpenRewrite Dispatcher] Waiting %v before retry...", sleepDuration)
				time.Sleep(sleepDuration) // Exponential backoff
			}
			continue
		}

		attemptDuration := time.Since(attemptStart)
		log.Printf("[OpenRewrite Dispatcher] Upload attempt %d SUCCESS in %v", i+1, attemptDuration)
		break
	}

	if lastErr != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: All upload attempts failed. Final error: %v", lastErr)
		return fmt.Errorf("failed to upload to storage after 3 attempts: %w", lastErr)
	}

	log.Printf("[OpenRewrite Dispatcher] Upload completed successfully")

	// Verify upload by checking if file exists in storage
	log.Printf("[OpenRewrite Dispatcher] Verifying upload by checking storage existence...")
	exists, err := d.storageClient.Exists(ctx, storageKey)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot verify upload existence: %v", err)
		return fmt.Errorf("failed to verify upload existence: %w", err)
	}
	if !exists {
		log.Printf("[OpenRewrite Dispatcher] ERROR: File does not exist in storage after upload: %s", storageKey)
		return fmt.Errorf("file does not exist in storage after upload: %s", storageKey)
	}
	log.Printf("[OpenRewrite Dispatcher] Storage existence verified successfully")

	// Get file info from storage for final verification
	log.Printf("[OpenRewrite Dispatcher] Retrieving storage file info for final verification...")
	data2, err := d.storageClient.Get(ctx, storageKey)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot retrieve file info from storage: %v", err)
		return fmt.Errorf("failed to retrieve file info from storage: %w", err)
	}
	storageSize := len(data2)
	log.Printf("[OpenRewrite Dispatcher] Storage file size: %d bytes", storageSize)

	if int64(storageSize) != fileInfo.Size() {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Storage size mismatch - local=%d, storage=%d", fileInfo.Size(), storageSize)
		return fmt.Errorf("storage size mismatch - local=%d, storage=%d", fileInfo.Size(), storageSize)
	}

	log.Printf("[OpenRewrite Dispatcher] ===== STORAGE UPLOAD SUCCESS =====")
	return nil
}

// downloadFromStorage downloads a file from the storage backend
func (d *OpenRewriteDispatcher) downloadFromStorage(ctx context.Context, storageKey, filePath string) error {
	data, err := d.storageClient.Get(ctx, storageKey)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}
