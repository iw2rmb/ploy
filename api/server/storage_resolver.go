package server

import (
    "fmt"

    cfgsvc "github.com/iw2rmb/ploy/internal/config"
    istorage "github.com/iw2rmb/ploy/internal/storage"
)

// resolveStorageFromConfigService requires the centralized config service and
// returns an error if it cannot resolve a storage client.
func resolveStorageFromConfigService(svc *cfgsvc.Service) (istorage.Storage, error) {
    if svc == nil {
        return nil, fmt.Errorf("config service is required for storage resolution")
    }
    cfg := svc.Get()
    if cfg == nil {
        return nil, fmt.Errorf("config service returned nil configuration")
    }
    s, err := cfg.CreateStorageClient()
    if err != nil || s == nil {
        if err == nil {
            err = fmt.Errorf("storage client is nil")
        }
        return nil, fmt.Errorf("create storage via config service: %w", err)
    }
    return s, nil
}
