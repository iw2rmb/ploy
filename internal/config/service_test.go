package config_test

import (
    "os"
    "testing"

    cfg "github.com/iw2rmb/ploy/internal/config"
    istorage "github.com/iw2rmb/ploy/internal/storage"
)

func TestNew_WithDefaults_LoadsAndGetReturnsClone(t *testing.T) {
    t.Parallel()

    defaults := &cfg.Config{
        App: cfg.AppConfig{
            Name:    "test-app",
            Version: "1.0.0",
        },
    }

    svc, err := cfg.New(cfg.WithDefaults(defaults))
    if err != nil {
        t.Fatalf("unexpected error creating service: %v", err)
    }

    got := svc.Get()
    if got.App.Name != "test-app" {
        t.Fatalf("expected app.name=test-app, got %q", got.App.Name)
    }

    // Mutate returned config and ensure original inside service is not affected
    got.App.Name = "mutated"

    got2 := svc.Get()
    if got2.App.Name != "test-app" {
        t.Fatalf("service returned mutated config copy, want 'test-app', got %q", got2.App.Name)
    }
}

func TestNew_WithEnvironment_OverridesDefaults(t *testing.T) {
    t.Parallel()

    // Ensure env is cleaned up
    os.Setenv("PLOY_APP_NAME", "override")
    t.Cleanup(func() { os.Unsetenv("PLOY_APP_NAME") })

    defaults := &cfg.Config{
        App: cfg.AppConfig{
            Name:    "default-name",
            Version: "1.0.0",
        },
    }

    svc, err := cfg.New(
        cfg.WithDefaults(defaults),
        cfg.WithEnvironment("PLOY_"),
    )
    if err != nil {
        t.Fatalf("unexpected error creating service: %v", err)
    }

    got := svc.Get()
    if got.App.Name != "override" {
        t.Fatalf("expected env override app.name=override, got %q", got.App.Name)
    }
}

func TestNew_WithFile_LoadsConfiguration(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    path := dir + "/config.yaml"
    content := []byte("app:\n  name: file-app\n  version: 2.0.0\n")
    if err := os.WriteFile(path, content, 0o644); err != nil {
        t.Fatalf("failed writing test config: %v", err)
    }

    svc, err := cfg.New(cfg.WithFile(path))
    if err != nil {
        t.Fatalf("unexpected error creating service: %v", err)
    }

    got := svc.Get()
    if got.App.Name != "file-app" || got.App.Version != "2.0.0" {
        t.Fatalf("config not loaded from file: %+v", got.App)
    }
}

func TestNew_WithFile_AndEnv_MergesWithOverride(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    path := dir + "/config.yaml"
    content := []byte("app:\n  name: file-app\n  version: 2.0.0\n")
    if err := os.WriteFile(path, content, 0o644); err != nil {
        t.Fatalf("failed writing test config: %v", err)
    }

    os.Setenv("PLOY_APP_NAME", "env-app")
    t.Cleanup(func() { os.Unsetenv("PLOY_APP_NAME") })

    svc, err := cfg.New(cfg.WithFile(path), cfg.WithEnvironment("PLOY_"))
    if err != nil {
        t.Fatalf("unexpected error creating service: %v", err)
    }

    got := svc.Get()
    if got.App.Name != "env-app" || got.App.Version != "2.0.0" {
        t.Fatalf("expected env override for name and keep version, got: %+v", got.App)
    }
}

func TestCreateStorageClient_MemoryProvider(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    path := dir + "/config.yaml"
    content := []byte("storage:\n  provider: memory\n")
    if err := os.WriteFile(path, content, 0o644); err != nil {
        t.Fatalf("failed writing test storage config: %v", err)
    }

    svc, err := cfg.New(cfg.WithFile(path))
    if err != nil {
        t.Fatalf("unexpected error creating service: %v", err)
    }

    // Minimal method to create storage from config
    storage, err := svc.Get().CreateStorageClient()
    if err != nil {
        t.Fatalf("unexpected error creating storage: %v", err)
    }
    if storage == nil {
        t.Fatalf("expected storage not nil")
    }
    // basic sanity: metrics exists and type conforms
    _ = storage.Metrics()

    // Ensure factory accepts provider field at least
    _ = istorage.StorageMetrics{}
}

func TestConfigurationService_ValidationFails(t *testing.T) {
    // This test ensures that when validators are provided,
    // invalid configuration causes New() to return an error.
    dir := t.TempDir()
    path := dir + "/config.yaml"
    // Missing region for s3 provider should fail validation
    content := []byte("storage:\n  provider: s3\n  bucket: test-bucket\n")
    if err := os.WriteFile(path, content, 0o644); err != nil {
        t.Fatalf("failed writing test config: %v", err)
    }

    _, err := cfg.New(
        cfg.WithFile(path),
        cfg.WithValidation(cfg.NewStructValidator()),
    )
    if err == nil {
        t.Fatalf("expected validation error, got nil")
    }
}

func TestConfigurationService_GetWithCache(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    path := dir + "/config.yaml"
    content := []byte("app:\n  name: cached-app\n")
    if err := os.WriteFile(path, content, 0o644); err != nil {
        t.Fatalf("failed writing config: %v", err)
    }

    svc, err := cfg.New(cfg.WithFile(path))
    if err != nil {
        t.Fatalf("unexpected error creating service: %v", err)
    }

    // First call should populate cache and report miss
    got1, fromCache := svc.GetWithCache("test")
    if fromCache {
        t.Fatalf("expected cache miss on first call")
    }
    if got1.App.Name != "cached-app" {
        t.Fatalf("expected app.name=cached-app, got %q", got1.App.Name)
    }

    // Second call with same key should be cache hit
    got2, fromCache2 := svc.GetWithCache("test")
    if !fromCache2 {
        t.Fatalf("expected cache hit on second call")
    }
    if got2.App.Name != "cached-app" {
        t.Fatalf("expected cached app.name=cached-app, got %q", got2.App.Name)
    }
}
