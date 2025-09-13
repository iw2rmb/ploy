package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	cfgsvc "github.com/iw2rmb/ploy/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: test-upload <binary-path>")
	}
	binaryPath := os.Args[1]

	svc, err := cfgsvc.New(
		cfgsvc.WithFile(os.Getenv("PLOY_STORAGE_CONFIG")),
		cfgsvc.WithEnvironment("PLOY_"),
		cfgsvc.WithValidation(cfgsvc.NewStructValidator()),
	)
	if err != nil {
		log.Fatalf("failed to init config: %v", err)
	}
	cfg := svc.Get()
	store, err := cfg.CreateStorageClient()
	if err != nil {
		log.Fatalf("failed to create storage: %v", err)
	}

	file, err := os.Open(binaryPath)
	if err != nil {
		log.Fatalf("failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	key := fmt.Sprintf("%s/%s", cfg.Storage.Bucket, "api-binaries/test/api")
	if err := store.Put(context.TODO(), key, file); err != nil {
		log.Fatalf("upload failed: %v", err)
	}

	reader, err := store.Get(context.TODO(), key)
	if err != nil {
		log.Fatalf("download failed: %v", err)
	}
	defer func() { _ = reader.Close() }()
	buf := make([]byte, 16)
	n, _ := io.ReadFull(reader, buf)
	fmt.Printf("Uploaded and retrieved %d bytes successfully\n", n)
}
