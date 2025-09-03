package server

import (
    "fmt"

    apiconfig "github.com/iw2rmb/ploy/api/config"
    cfgsvc "github.com/iw2rmb/ploy/internal/config"
    istorage "github.com/iw2rmb/ploy/internal/storage"
)

// resolveStorageFromConfigService prefers the internal/config service when provided,
// falling back to the existing factory path using the legacy api/config helper.
func resolveStorageFromConfigService(svc *cfgsvc.Service, fallbackConfigPath string) (istorage.Storage, error) {
    if svc != nil {
        cfg := svc.Get()
        if cfg != nil {
            s, err := cfg.CreateStorageClient()
            if err == nil && s != nil {
                return s, nil
            }
            if err != nil {
                return nil, fmt.Errorf("create storage via config service: %w", err)
            }
        }
    }
    if fallbackConfigPath == "" {
        return nil, fmt.Errorf("no config service and empty fallback path")
    }
    // Fallback to existing factory helper in api/config
    return apiconfig.CreateStorageFromFactory(fallbackConfigPath)
}

