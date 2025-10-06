package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
	lanescatalog "github.com/iw2rmb/ploy/lanes"
)

type runnerInvoker interface {
	Run(ctx context.Context, opts runner.Options) error
}

type runnerInvokerFunc func(context.Context, runner.Options) error

// Run executes the injected runner function implementation.
func (f runnerInvokerFunc) Run(ctx context.Context, opts runner.Options) error {
	return f(ctx, opts)
}

type eventsFactoryFunc func(tenant string) (runner.EventsClient, error)

type gridFactoryFunc func() (runner.GridClient, error)

type laneRegistry interface {
	Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error)
}

type laneRegistryLoaderFunc func(dir string) (laneRegistry, error)

type knowledgeBaseAdvisorLoaderFunc func(path string) (mods.Advisor, error)

type snapshotRegistry interface {
	Plan(ctx context.Context, name string) (snapshots.PlanReport, error)
	Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error)
}

type snapshotRegistryLoaderFunc func(dir string) (snapshotRegistry, error)

type manifestCompilerLoaderFunc func(dir string) (runner.ManifestCompiler, error)

type environmentService interface {
	Materialize(ctx context.Context, req environments.Request) (environments.Result, error)
}

type environmentFactoryFunc func(l laneRegistry, s snapshotRegistry) (environmentService, error)

type asterLocatorLoaderFunc func(dir string) (aster.Locator, error)

type workspacePreparerFactoryFunc func() (runner.WorkspacePreparer, error)

type laneCacheComposer struct {
	lanes laneRegistry
}

const workflowSDKStateEnv = "GRID_WORKFLOW_SDK_STATE_DIR"

// Compose produces cache keys by delegating to the configured lane registry.
func (c laneCacheComposer) Compose(ctx context.Context, req runner.CacheComposeRequest) (string, error) {
	_ = ctx
	if c.lanes == nil {
		return "", fmt.Errorf("lane registry unavailable")
	}
	manifestVersion := req.Stage.Constraints.Manifest.Manifest.Version
	desc, err := c.lanes.Describe(req.Stage.Lane, lanes.DescribeOptions{
		ManifestVersion: manifestVersion,
		AsterToggles:    req.Stage.Aster.Toggles,
	})
	if err != nil {
		return "", err
	}
	return desc.CacheKey, nil
}

var (
	runnerExecutor           runnerInvoker                = runnerInvokerFunc(runner.Run)
	eventsFactory            eventsFactoryFunc            = defaultEventsFactory
	gridFactory              gridFactoryFunc              = defaultGridFactory
	workspacePreparerFactory workspacePreparerFactoryFunc = defaultWorkspacePreparerFactory

	newJetStreamClient = contracts.NewJetStreamClient
)

var (
	knowledgeBaseAdvisorLoader knowledgeBaseAdvisorLoaderFunc = defaultKnowledgeBaseAdvisorLoader
)

type integrationConfig struct {
	APIEndpoint   string
	JetStreamURL  string
	JetStreamURLs []string
	IPFSGateway   string
	Features      map[string]string
	Version       string
}

// FeatureEnabled reports whether the named discovery feature is marked as enabled.
func (cfg integrationConfig) FeatureEnabled(name string) bool {
	if len(cfg.Features) == 0 {
		return false
	}
	value, ok := cfg.Features[name]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "enabled")
}

var (
	discoveryCache     = newClusterInfoCache()
	fetchClusterInfoFn = fetchClusterInfo
)

// defaultEventsFactory builds an events client, preferring JetStream when configured.
func defaultEventsFactory(tenant string) (runner.EventsClient, error) {
	trimmedTenant := strings.TrimSpace(tenant)
	if trimmedTenant == "" {
		return nil, fmt.Errorf("tenant is required for events client")
	}
	cfg, _ := resolveIntegrationConfig(context.Background())
	jetstreamURL := strings.TrimSpace(cfg.JetStreamURL)
	if jetstreamURL != "" {
		client, err := newJetStreamClient(contracts.JetStreamOptions{
			URL:    jetstreamURL,
			Tenant: trimmedTenant,
		})
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	return contracts.NewInMemoryBus(trimmedTenant), nil
}

// defaultGridFactory returns either an in-memory grid client or the configured endpoint client.
func defaultGridFactory() (runner.GridClient, error) {
	endpoint := strings.TrimSpace(os.Getenv("GRID_ENDPOINT"))
	if endpoint == "" {
		return runner.NewInMemoryGrid(), nil
	}
	stateDir, err := ensureWorkflowSDKStateDir()
	if err != nil {
		return nil, err
	}

	options := grid.Options{
		Endpoint:           endpoint,
		StreamOptions:      gridStreamOptions(),
		CursorStoreFactory: grid.NewCursorStoreFactory(stateDir),
	}
	client, err := grid.NewClient(options)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func resolveLaneDirectories(dir string) []string {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" || strings.EqualFold(trimmed, "auto") {
		return []string{"lanes"}
	}
	return []string{trimmed}
}

func loadEmbeddedLaneRegistry() (laneRegistry, error) {
	embeddedLaneOnce.Do(func() {
		tempDir, err := os.MkdirTemp("", "ploy-lanes-")
		if err != nil {
			embeddedLaneErr = err
			return
		}
		if err := copyEmbeddedLaneCatalog(tempDir); err != nil {
			embeddedLaneErr = err
			return
		}
		embeddedLaneRegistry, embeddedLaneErr = lanes.LoadDirectory(tempDir)
	})
	return embeddedLaneRegistry, embeddedLaneErr
}

func copyEmbeddedLaneCatalog(dest string) error {
	return fs.WalkDir(lanescatalog.Catalog, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".toml") {
			return nil
		}
		data, err := lanescatalog.Catalog.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Base(path))
		return os.WriteFile(target, data, 0o644)
	})
}

var (
	embeddedLaneOnce     sync.Once
	embeddedLaneRegistry laneRegistry
	embeddedLaneErr      error

	laneRegistryLoader laneRegistryLoaderFunc = func(dir string) (laneRegistry, error) {
		trimmed := strings.TrimSpace(dir)
		defaultSearch := trimmed == "" || strings.EqualFold(trimmed, "auto")
		candidates := resolveLaneDirectories(dir)
		var firstErr error
		for _, candidate := range candidates {
			registry, err := lanes.LoadDirectory(candidate)
			if err == nil {
				return registry, nil
			}
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, fs.ErrNotExist) {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			return nil, err
		}
		if defaultSearch {
			registry, err := loadEmbeddedLaneRegistry()
			if err == nil {
				return registry, nil
			}
			if firstErr != nil {
				return nil, fmt.Errorf("load lanes from disk: %v; embedded catalog error: %w", firstErr, err)
			}
			return nil, err
		}
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("no lane directories found (searched %v)", candidates)
	}
	laneConfigDir = ""

	snapshotRegistryLoader snapshotRegistryLoaderFunc = func(dir string) (snapshotRegistry, error) {
		opts := snapshots.LoadOptions{}
		cfg, _ := resolveIntegrationConfig(context.Background())
		gateway := strings.TrimSpace(cfg.IPFSGateway)
		if gateway != "" {
			publisher, err := snapshots.NewIPFSGatewayPublisher(gateway, snapshots.IPFSGatewayOptions{Pin: true})
			if err != nil {
				return nil, err
			}
			opts.ArtifactPublisher = publisher
		}
		jetstreamURL := strings.TrimSpace(cfg.JetStreamURL)
		if jetstreamURL != "" {
			metadataPublisher, err := snapshots.NewJetStreamMetadataPublisher(jetstreamURL, snapshots.JetStreamMetadataOptions{})
			if err != nil {
				return nil, err
			}
			opts.MetadataPublisher = metadataPublisher
		}
		return snapshots.LoadDirectory(dir, opts)
	}
	snapshotConfigDir = "configs/snapshots"

	manifestRegistryLoader manifestCompilerLoaderFunc = func(dir string) (runner.ManifestCompiler, error) {
		registry, err := manifests.LoadDirectory(dir)
		if err != nil {
			return nil, err
		}
		return registryCompiler{registry: registry}, nil
	}
	manifestConfigDir = "configs/manifests"

	knowledgeBaseCatalogPath = "configs/knowledge-base/catalog.json"

	asterLocatorLoader asterLocatorLoaderFunc = func(dir string) (aster.Locator, error) {
		return aster.NewFilesystemLocator(dir)
	}
	asterConfigDir = "configs/aster"

	environmentServiceFactory environmentFactoryFunc = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		if l == nil {
			return nil, fmt.Errorf("environment lane registry missing")
		}
		if s == nil {
			return nil, fmt.Errorf("environment snapshot registry missing")
		}
		hydrator := environments.NewInMemoryHydrator()
		return environments.NewService(environments.ServiceOptions{
			Lanes:     l,
			Snapshots: s,
			Hydrator:  hydrator,
		}), nil
	}

	manifestSchemaPath = "docs/schemas/integration_manifest.schema.json"
)

// defaultKnowledgeBaseAdvisorLoader constructs a knowledge base advisor when a catalog is available.
func defaultKnowledgeBaseAdvisorLoader(path string) (mods.Advisor, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil
	}
	catalog, err := knowledgebase.LoadCatalogFile(trimmed)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	advisor, err := knowledgebase.NewAdvisor(knowledgebase.Options{Catalog: catalog})
	if err != nil {
		return nil, err
	}
	return advisor, nil
}

func ensureWorkflowSDKStateDir() (string, error) {
	if existing := strings.TrimSpace(os.Getenv(workflowSDKStateEnv)); existing != "" {
		if err := os.MkdirAll(existing, 0o755); err != nil {
			return "", fmt.Errorf("prepare workflow sdk state dir: %w", err)
		}
		return existing, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	stateDir := filepath.Join(configDir, "ploy", "grid")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare workflow sdk state dir: %w", err)
	}
	if err := os.Setenv(workflowSDKStateEnv, stateDir); err != nil {
		return "", fmt.Errorf("set %s: %w", workflowSDKStateEnv, err)
	}
	return stateDir, nil
}

func gridStreamOptions() helper.StreamOptions {
	return helper.StreamOptions{
		HeartbeatInterval: 20 * time.Second,
		MinBackoff:        200 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
	}
}

func resolveIntegrationConfig(ctx context.Context) (integrationConfig, error) {
	endpoint := strings.TrimSpace(os.Getenv("GRID_ENDPOINT"))

	if endpoint == "" {
		return integrationConfig{Features: map[string]string{}}, nil
	}

	info, err := loadClusterInfo(ctx, endpoint)
	if err != nil {
		fallback := integrationConfig{
			APIEndpoint: sanitizeAPIEndpoint(endpoint),
			Features:    map[string]string{},
		}
		return fallback, err
	}

	cfg := integrationConfig{
		APIEndpoint:   firstNonEmpty(info.APIEndpoint, sanitizeAPIEndpoint(endpoint)),
		JetStreamURLs: append([]string(nil), info.JetStreamURLs...),
		IPFSGateway:   strings.TrimSpace(info.IPFSGateway),
		Features:      copyFeaturesMap(info.Features),
		Version:       strings.TrimSpace(info.Version),
	}

	cfg.JetStreamURL = firstJetStreamRoute(cfg.JetStreamURLs)

	if cfg.Features == nil {
		cfg.Features = map[string]string{}
	}

	return cfg, nil
}

type clusterInfo struct {
	APIEndpoint   string
	JetStreamURLs []string
	IPFSGateway   string
	Features      map[string]string
	Version       string
}

func (c clusterInfo) clone() clusterInfo {
	clone := clusterInfo{
		APIEndpoint: strings.TrimSpace(c.APIEndpoint),
		IPFSGateway: strings.TrimSpace(c.IPFSGateway),
		Features:    copyFeaturesMap(c.Features),
		Version:     strings.TrimSpace(c.Version),
	}
	clone.JetStreamURLs = normalizeJetStreamRoutes(c.JetStreamURLs)
	return clone
}

type clusterInfoCache struct {
	mu      sync.Mutex
	entries map[string]clusterInfo
}

func newClusterInfoCache() *clusterInfoCache {
	return &clusterInfoCache{entries: make(map[string]clusterInfo)}
}

func (c *clusterInfoCache) get(endpoint string) (clusterInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, ok := c.entries[endpoint]
	if !ok {
		return clusterInfo{}, false
	}
	return info.clone(), true
}

func (c *clusterInfoCache) set(endpoint string, info clusterInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[endpoint] = info.clone()
}

func loadClusterInfo(ctx context.Context, endpoint string) (clusterInfo, error) {
	if info, ok := discoveryCache.get(endpoint); ok {
		return info, nil
	}

	info, err := fetchClusterInfoWithRetry(ctx, endpoint)
	if err != nil {
		return clusterInfo{}, err
	}

	discoveryCache.set(endpoint, info)
	return info, nil
}

func fetchClusterInfoWithRetry(ctx context.Context, endpoint string) (clusterInfo, error) {
	const attempts = 3
	baseBackoff := 100 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		info, err := fetchClusterInfoFn(ctx, endpoint)
		if err == nil {
			return info, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return clusterInfo{}, ctx.Err()
		default:
		}
		if i < attempts-1 {
			time.Sleep(time.Duration(i+1) * baseBackoff)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("cluster discovery failed")
	}
	return clusterInfo{}, lastErr
}

// fetchClusterInfo retrieves cluster discovery data from the provided endpoint.
func fetchClusterInfo(ctx context.Context, endpoint string) (info clusterInfo, err error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return clusterInfo{}, fmt.Errorf("grid endpoint required for discovery")
	}

	discoveryURL := fmt.Sprintf("%s/v1/cluster/info", strings.TrimRight(trimmed, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return clusterInfo{}, fmt.Errorf("create discovery request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return clusterInfo{}, fmt.Errorf("fetch discovery info: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close discovery response body: %w", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return clusterInfo{}, fmt.Errorf("discovery request failed: %s", resp.Status)
	}

	var payload struct {
		APIEndpoint   string            `json:"api_endpoint"`
		JetStreamURLs []string          `json:"jetstream_urls"`
		IPFSGateway   string            `json:"ipfs_gateway"`
		Features      map[string]string `json:"features"`
		Version       string            `json:"version"`
	}
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return clusterInfo{}, fmt.Errorf("decode discovery response: %w", err)
	}

	return clusterInfo{
		APIEndpoint:   strings.TrimSpace(payload.APIEndpoint),
		JetStreamURLs: normalizeJetStreamRoutes(payload.JetStreamURLs),
		IPFSGateway:   strings.TrimSpace(payload.IPFSGateway),
		Features:      copyFeaturesMap(payload.Features),
		Version:       strings.TrimSpace(payload.Version),
	}, nil
}

func normalizeJetStreamRoutes(routes []string) []string {
	if len(routes) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(routes))
	for _, route := range routes {
		trimmed := strings.TrimSpace(route)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func firstJetStreamRoute(routes []string) string {
	for _, route := range routes {
		trimmed := strings.TrimSpace(route)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func copyFeaturesMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		dst[trimmedKey] = strings.TrimSpace(value)
	}
	if len(dst) == 0 {
		return map[string]string{}
	}
	return dst
}

func sanitizeAPIEndpoint(value string) string {
	trimmed := strings.TrimSpace(value)
	return strings.TrimRight(trimmed, "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type registryCompiler struct {
	registry *manifests.Registry
}

// Compile resolves a manifest reference using the loaded registry instance.
func (r registryCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	if r.registry == nil {
		return manifests.Compilation{}, fmt.Errorf("compile manifest: registry missing")
	}
	comp, err := r.registry.Compile(manifests.CompileOptions{Name: ref.Name, Version: ref.Version})
	if err != nil {
		return manifests.Compilation{}, err
	}
	return comp, nil
}
