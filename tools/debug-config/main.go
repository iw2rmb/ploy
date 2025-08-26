package main

import (
	"fmt"
	"log"

	"github.com/iw2rmb/ploy/api/config"
	"github.com/iw2rmb/ploy/internal/storage"
)

func main() {
	configPath := config.GetStorageConfigPath()
	fmt.Printf("Config path: %s\n", configPath)
	
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	fmt.Printf("Storage config loaded:\n")
	fmt.Printf("  Provider: %s\n", cfg.Storage.Provider)
	fmt.Printf("  Master: %s\n", cfg.Storage.Master)
	fmt.Printf("  Filer: %s\n", cfg.Storage.Filer)
	fmt.Printf("  Collection: %s\n", cfg.Storage.Collection)
	fmt.Printf("  Replication: %q\n", cfg.Storage.Replication)
	fmt.Printf("  Timeout: %d\n", cfg.Storage.Timeout)
	
	seaweedfsConfig := storage.SeaweedFSConfig{
		Master:      cfg.Storage.Master,
		Filer:       cfg.Storage.Filer,
		Collection:  cfg.Storage.Collection,
		Replication: cfg.Storage.Replication,
		Timeout:     cfg.Storage.Timeout,
		DataCenter:  cfg.Storage.DataCenter,
		Rack:        cfg.Storage.Rack,
	}
	
	fmt.Printf("\nSeaweedFS config created:\n")
	fmt.Printf("  Master: %s\n", seaweedfsConfig.Master)
	fmt.Printf("  Filer: %s\n", seaweedfsConfig.Filer)
	fmt.Printf("  Collection: %s\n", seaweedfsConfig.Collection)
	fmt.Printf("  Replication: %q\n", seaweedfsConfig.Replication)
	fmt.Printf("  Timeout: %d\n", seaweedfsConfig.Timeout)
	
	client, err := storage.NewSeaweedFSClient(seaweedfsConfig)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	
	fmt.Printf("\nClient created successfully\n")
	_ = client
}