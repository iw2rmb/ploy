package config_test

import (
    "os"
    "testing"
    "time"

    cfg "github.com/iw2rmb/ploy/internal/config"
    istorage "github.com/iw2rmb/ploy/internal/storage"
    "github.com/stretchr/testify/require"
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

func TestCreateStorageClient_SeaweedFSMapping(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    path := dir + "/config.yaml"
    content := []byte("storage:\n  provider: seaweedfs\n  endpoint: http://localhost:9333\n  bucket: test-collection\n")
    require.NoError(t, os.WriteFile(path, content, 0o644))

    svc, err := cfg.New(cfg.WithFile(path), cfg.WithValidation(cfg.NewStructValidator()))
    require.NoError(t, err)

    stor, err := svc.Get().CreateStorageClient()
    require.NoError(t, err)
    require.NotNil(t, stor)
}

func TestCreateStorageClient_WithRetryAndCache(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    path := dir + "/config.yaml"
    content := []byte("storage:\n  provider: memory\n  retry:\n    enabled: true\n    max_attempts: 2\n    initial_delay: 50ms\n    max_delay: 200ms\n    backoff_multiplier: 1.5\n  cache:\n    enabled: true\n    max_size: 10\n    ttl: 1s\n")
    require.NoError(t, os.WriteFile(path, content, 0o644))

    svc, err := cfg.New(cfg.WithFile(path))
    require.NoError(t, err)

    stor, err := svc.Get().CreateStorageClient()
    require.NoError(t, err)
    require.NotNil(t, stor)
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

func TestStructValidator_SeaweedFSEndpointRequired(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    path := dir + "/config.yaml"
    // Provider seaweedfs but no endpoint should fail validation
    content := []byte("storage:\n  provider: seaweedfs\n  bucket: test\n")
    require.NoError(t, os.WriteFile(path, content, 0o644))

    _, err := cfg.New(
        cfg.WithFile(path),
        cfg.WithValidation(cfg.NewStructValidator()),
    )
    if err == nil {
        t.Fatalf("expected validation error for missing seaweedfs endpoint, got nil")
    }
}

func TestHotReload_FromFileChange(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    path := dir + "/config.yaml"
    // initial config
    require.NoError(t, os.WriteFile(path, []byte("app:\n  name: v1\n"), 0o644))

    svc, err := cfg.New(
        cfg.WithFile(path),
        cfg.WithHotReload(25*time.Millisecond),
    )
    require.NoError(t, err)

    got := svc.Get()
    require.Equal(t, "v1", got.App.Name)

    // Watch callback to observe change
    changed := make(chan struct{}, 1)
    svc.Watch(func(c *cfg.Config) {
        if c.App.Name == "v2" {
            changed <- struct{}{}
        }
    })

    // mutate the file
    require.NoError(t, os.WriteFile(path, []byte("app:\n  name: v2\n"), 0o644))

    // wait for callback or timeout
    select {
    case <-changed:
        // ok
    case <-time.After(500 * time.Millisecond):
        t.Fatalf("hot reload did not trigger within timeout")
    }

    // ensure Get reflects new value
    got2 := svc.Get()
    require.Equal(t, "v2", got2.App.Name)
}

func TestEnvironmentOverrides_StorageProviderAndEndpoint(t *testing.T) {

    dir := t.TempDir()
    path := dir + "/config.yaml"
    // File sets provider to memory; env should override to seaweedfs and set endpoint
    content := []byte("app:\n  name: test-app\nstorage:\n  provider: memory\n")
    require.NoError(t, os.WriteFile(path, content, 0o644))

    // Set environment overrides
    t.Setenv("PLOY_STORAGE_PROVIDER", "seaweedfs")
    t.Setenv("PLOY_STORAGE_ENDPOINT", "http://localhost:9333")

    svc, err := cfg.New(cfg.WithFile(path), cfg.WithEnvironment("PLOY_"), cfg.WithValidation(cfg.NewStructValidator()))
    require.NoError(t, err)

    cfgNow := svc.Get()
    require.Equal(t, "seaweedfs", cfgNow.Storage.Provider)
    require.Equal(t, "http://localhost:9333", cfgNow.Storage.Endpoint)

    // Ensure we can create a storage client with the overridden settings
    st, err := cfgNow.CreateStorageClient()
    require.NoError(t, err)
    require.NotNil(t, st)
}
