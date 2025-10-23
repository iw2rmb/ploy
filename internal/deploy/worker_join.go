package deploy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/registry"
)

// WorkerJoinOptions configure the worker onboarding flow.
type WorkerJoinOptions struct {
	ClusterID     string
	WorkerID      string
	Address       string
	Labels        map[string]string
	HealthProbes  []WorkerHealthProbe
	HealthChecker HealthChecker
	Clock         func() time.Time
	DryRun        bool
}

// WorkerHealthProbe describes a health endpoint that must succeed during onboarding.
type WorkerHealthProbe struct {
	Name         string
	Endpoint     string
	ExpectStatus int
}

// WorkerJoinResult summarises the onboarding outcome.
type WorkerJoinResult struct {
	ClusterID   string
	Descriptor  registry.WorkerDescriptor
	Certificate LeafCertificate
	Health      []registry.WorkerProbeResult
	DryRun      bool
}

// HealthChecker evaluates worker health probes.
type HealthChecker interface {
	Run(ctx context.Context, workerID, address string, probes []WorkerHealthProbe) ([]registry.WorkerProbeResult, error)
}

// HTTPHealthChecker executes HTTP GET probes against worker endpoints.
type HTTPHealthChecker struct {
	Client *http.Client
	Clock  func() time.Time
}

// Run executes the configured probes, marking failures on non-successful HTTP responses.
func (h *HTTPHealthChecker) Run(ctx context.Context, workerID, address string, probes []WorkerHealthProbe) ([]registry.WorkerProbeResult, error) {
	if len(probes) == 0 {
		return nil, nil
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	var now func() time.Time
	if h.Clock != nil {
		now = func() time.Time { return h.Clock().UTC() }
	} else {
		now = func() time.Time { return time.Now().UTC() }
	}
	results := make([]registry.WorkerProbeResult, 0, len(probes))
	for _, probe := range probes {
		checkedAt := now()
		result := registry.WorkerProbeResult{
			Name:      strings.TrimSpace(probe.Name),
			Endpoint:  strings.TrimSpace(probe.Endpoint),
			CheckedAt: checkedAt,
		}
		if result.Name == "" {
			result.Name = "probe"
		}
		if result.Endpoint == "" {
			result.Passed = false
			result.Message = "empty probe endpoint"
			results = append(results, result)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, result.Endpoint, nil)
		if err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("build request: %v", err)
			results = append(results, result)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			result.Passed = false
			result.Message = err.Error()
			results = append(results, result)
			continue
		}
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		result.StatusCode = resp.StatusCode
		expect := probe.ExpectStatus
		if expect == 0 {
			expect = http.StatusOK
		}
		if resp.StatusCode != expect {
			result.Passed = false
			result.Message = fmt.Sprintf("expected status %d, got %d", expect, resp.StatusCode)
		} else {
			result.Passed = true
		}
		results = append(results, result)
	}
	return results, nil
}

// RunWorkerJoin orchestrates worker registration, certificate issuance, and health verification.
func RunWorkerJoin(ctx context.Context, client *clientv3.Client, opts WorkerJoinOptions) (WorkerJoinResult, error) {
	if client == nil {
		return WorkerJoinResult{}, errors.New("deploy: etcd client required")
	}
	clusterID := strings.TrimSpace(opts.ClusterID)
	if clusterID == "" {
		return WorkerJoinResult{}, errors.New("deploy: cluster id required")
	}
	workerID := strings.TrimSpace(opts.WorkerID)
	if workerID == "" {
		return WorkerJoinResult{}, errors.New("deploy: worker id required")
	}
	address := strings.TrimSpace(opts.Address)
	if address == "" {
		return WorkerJoinResult{}, errors.New("deploy: worker address required")
	}

	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	now := clock().UTC()

	manager, err := NewCARotationManager(client, clusterID)
	if err != nil {
		return WorkerJoinResult{}, err
	}

	reg, err := registry.NewWorkerRegistry(client, clusterID)
	if err != nil {
		return WorkerJoinResult{}, err
	}

	exists, err := reg.Exists(ctx, workerID)
	if err != nil {
		return WorkerJoinResult{}, err
	}
	if exists {
		return WorkerJoinResult{}, registry.ErrWorkerExists
	}

	state, err := manager.State(ctx)
	if err != nil {
		return WorkerJoinResult{}, err
	}

	httpClient, err := newWorkerHealthHTTPClient(state.CurrentCA.CertificatePEM)
	if err != nil {
		return WorkerJoinResult{}, err
	}

	for _, existing := range state.Nodes.Workers {
		if existing == workerID {
			return WorkerJoinResult{}, registry.ErrWorkerExists
		}
	}

	prober := opts.HealthChecker
	if prober == nil {
		prober = &HTTPHealthChecker{
			Client: httpClient,
			Clock:  clock,
		}
	} else if hc, ok := prober.(*HTTPHealthChecker); ok {
		if hc.Client == nil {
			hc.Client = httpClient
		}
		if hc.Clock == nil {
			hc.Clock = clock
		}
	}

	if opts.DryRun {
		results, err := prober.Run(ctx, workerID, address, opts.HealthProbes)
		if err != nil {
			return WorkerJoinResult{}, err
		}
		if failed, message := firstFailedProbe(results); failed {
			return WorkerJoinResult{
				ClusterID: clusterID,
				DryRun:    true,
				Health:    results,
			}, fmt.Errorf("deploy: health probe failed: %s", message)
		}
		descriptor := registry.WorkerDescriptor{
			ID:           workerID,
			Address:      address,
			Labels:       cloneLabels(opts.Labels),
			RegisteredAt: now,
			Status: registry.WorkerStatus{
				Phase:     registry.WorkerPhaseRegistering,
				CheckedAt: now,
				Probes:    results,
				Message:   "dry run preview",
			},
		}
		return WorkerJoinResult{
			ClusterID:  clusterID,
			Descriptor: descriptor,
			Health:     results,
			DryRun:     true,
		}, nil
	}

	descriptor := registry.WorkerDescriptor{
		ID:           workerID,
		Address:      address,
		Labels:       cloneLabels(opts.Labels),
		RegisteredAt: now,
		Status: registry.WorkerStatus{
			Phase:     registry.WorkerPhaseRegistering,
			CheckedAt: now,
			Message:   "registration in progress",
		},
	}

	record, err := reg.Register(ctx, descriptor)
	if err != nil {
		return WorkerJoinResult{}, err
	}

	cert, err := manager.IssueWorkerCertificate(ctx, workerID, now)
	if err != nil {
		_ = reg.Delete(ctx, workerID)
		return WorkerJoinResult{}, err
	}

	descriptor.CertificateVersion = cert.Version

	results, err := prober.Run(ctx, workerID, address, opts.HealthProbes)
	if err != nil {
		_ = manager.RemoveWorker(ctx, workerID)
		_ = reg.Delete(ctx, workerID)
		return WorkerJoinResult{}, err
	}
	if failed, message := firstFailedProbe(results); failed {
		_ = manager.RemoveWorker(ctx, workerID)
		_ = reg.Delete(ctx, workerID)
		return WorkerJoinResult{
			ClusterID: clusterID,
			Health:    results,
		}, fmt.Errorf("deploy: health probe failed: %s", message)
	}

	descriptor.Status = registry.WorkerStatus{
		Phase:     registry.WorkerPhaseReady,
		CheckedAt: clock().UTC(),
		Message:   "worker registered",
		Probes:    results,
	}

	updated, err := reg.Update(ctx, record, descriptor)
	if err != nil {
		_ = manager.RemoveWorker(ctx, workerID)
		_ = reg.Delete(ctx, workerID)
		return WorkerJoinResult{}, err
	}

	return WorkerJoinResult{
		ClusterID:   clusterID,
		Descriptor:  updated.Descriptor,
		Certificate: cert,
		Health:      results,
		DryRun:      false,
	}, nil
}

func firstFailedProbe(results []registry.WorkerProbeResult) (bool, string) {
	for _, result := range results {
		if !result.Passed {
			if result.Message != "" {
				return true, fmt.Sprintf("%s: %s", result.Name, result.Message)
			}
			return true, result.Name
		}
	}
	return false, ""
}

func cloneLabels(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		dst[key] = strings.TrimSpace(v)
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func newWorkerHealthHTTPClient(caCertificatePEM string) (*http.Client, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if caCertificatePEM != "" {
		if ok := roots.AppendCertsFromPEM([]byte(caCertificatePEM)); !ok {
			return nil, errors.New("deploy: add cluster CA to health check client")
		}
	}
	transport, err := cloneDefaultTransport()
	if err != nil {
		return nil, err
	}
	tlsCfg := ensureTLSConfig(transport.TLSClientConfig)
	tlsCfg.RootCAs = roots
	transport.TLSClientConfig = tlsCfg
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}, nil
}

func cloneDefaultTransport() (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("deploy: default transport is not *http.Transport")
	}
	return base.Clone(), nil
}

func ensureTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	cloned := cfg.Clone()
	if cloned.MinVersion == 0 {
		cloned.MinVersion = tls.VersionTLS12
	}
	return cloned
}
