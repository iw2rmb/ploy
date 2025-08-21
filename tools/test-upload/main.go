package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ploy/ploy/controller/config"
	"github.com/ploy/ploy/internal/storage"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: test-upload <binary-path>")
	}

	binaryPath := os.Args[1]

	// Load config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create client
	seaweedfsConfig := storage.SeaweedFSConfig{
		Master:      cfg.Storage.Master,
		Filer:       cfg.Storage.Filer,
		Collection:  cfg.Storage.Collection,
		Replication: cfg.Storage.Replication,
		Timeout:     cfg.Storage.Timeout,
		DataCenter:  cfg.Storage.DataCenter,
		Rack:        cfg.Storage.Rack,
	}

	client, err := storage.NewSeaweedFSClient(seaweedfsConfig)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Open binary file
	file, err := os.Open(binaryPath)
	if err != nil {
		log.Fatalf("Failed to open binary: %v", err)
	}
	defer file.Close()

	// Upload to simple path
	bucket := cfg.Storage.Collections.Artifacts
	key := "controller-binaries/test/controller"

	fmt.Printf("Uploading %s to %s/%s...\n", binaryPath, bucket, key)
	result, err := client.PutObject(bucket, key, file, "application/octet-stream")
	if err != nil {
		log.Fatalf("Upload failed: %v", err)
	}

	fmt.Printf("Upload successful!\n")
	fmt.Printf("ETag: %s\n", result.ETag)
	fmt.Printf("Location: %s\n", result.Location)
	fmt.Printf("Size: %d\n", result.Size)

	// Test retrieval
	fmt.Printf("\nTesting retrieval...\n")
	reader, err := client.GetObject(bucket, key)
	if err != nil {
		log.Fatalf("Retrieval failed: %v", err)
	}
	defer reader.Close()

	// Read first few bytes to verify
	buf := make([]byte, 100)
	n, err := reader.Read(buf)
	if err != nil && !strings.Contains(err.Error(), "EOF") {
		log.Fatalf("Read failed: %v", err)
	}

	fmt.Printf("Retrieved %d bytes successfully\n", n)
	fmt.Printf("First few bytes: %x...\n", buf[:min(16, n)])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}