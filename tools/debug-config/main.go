package main

import (
    "fmt"
    "log"

    cfgsvc "github.com/iw2rmb/ploy/internal/config"
)

func main() {
    svc, err := cfgsvc.New(
        cfgsvc.WithFile(cfgsvc.Getenv("PLOY_STORAGE_CONFIG", "")),
        cfgsvc.WithEnvironment("PLOY_"),
        cfgsvc.WithValidation(cfgsvc.NewStructValidator()),
    )
    if err != nil { log.Fatalf("failed to init config: %v", err) }
    cfg := svc.Get()
    fmt.Printf("Storage configuration (internal):\n  Provider: %s\n  Endpoint: %s\n  Bucket: %s\n",
        cfg.Storage.Provider, cfg.Storage.Endpoint, cfg.Storage.Bucket)
    if _, err := cfg.CreateStorageClient(); err != nil {
        log.Fatalf("failed to create storage from config: %v", err)
    }
    fmt.Println("Storage client created successfully")
}
