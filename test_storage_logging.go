package main

import (
	"context"
	"fmt"
	"log"
	"time"

	arfStorage "github.com/iw2rmb/ploy/api/arf/storage"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

func main() {
	log.Println("Starting SeaweedFS storage adapter logging test...")

	// Create SeaweedFS client configuration  
	config := internalStorage.SeaweedFSConfig{
		Master:      "seaweedfs-master.service.consul:9333",
		Filer:       "seaweedfs-filer.service.consul:8888", 
		Collection:  "ploy-artifacts",
		Replication: "001",
		Timeout:     30,
	}

	// Create SeaweedFS client
	seaweedfsClient, err := internalStorage.NewSeaweedFSClient(config)
	if err != nil {
		log.Fatalf("Failed to create SeaweedFS client: %v", err)
	}

	// Create storage adapter (same interface as dispatcher uses)
	storageAdapter := arfStorage.NewInternalStorageAdapter(seaweedfsClient)

	// Test data with timestamp to avoid conflicts
	testKey := fmt.Sprintf("openrewrite/test-%d/input.tar", time.Now().Unix())
	testData := []byte("This is a test tar file for enhanced logging verification")

	fmt.Printf("Testing StorageAdapter Put with key: %s\n", testKey)

	// Upload test file using storage adapter interface (same as dispatcher)
	ctx := context.Background()
	err = storageAdapter.Put(ctx, testKey, testData)
	if err != nil {
		log.Fatalf("Storage adapter Put failed: %v", err)
	}

	fmt.Printf("Upload successful!\n")

	// Test existence check
	fmt.Printf("Testing StorageAdapter Exists with key: %s\n", testKey)
	exists, err := storageAdapter.Exists(ctx, testKey)
	if err != nil {
		log.Fatalf("Storage adapter Exists failed: %v", err)
	}

	fmt.Printf("Exists check result: %v\n", exists)

	if exists {
		fmt.Println("Storage adapter test completed successfully!")
	} else {
		log.Fatalf("File does not exist after upload!")
	}
}