package controlplane

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

const (
	// DefaultPort is the HTTPS listener used by ployd control-plane nodes.
	DefaultPort        = 8443
	controlPlaneURLEnv = "PLOY_CONTROL_PLANE_URL"
)

var (
	defaultHTTPTimeout   = 15 * time.Second
)

// Options controls how ResolveTarget / ResolveHTTP derive the control-plane endpoint.
type Options struct {
	ClusterID string
	Endpoint  string
}

// Target captures the resolved control-plane endpoint and backing descriptor metadata.
type Target struct {
	ClusterID  string
	BaseURL    *url.URL
	Descriptor *config.Descriptor
}

// ResolveTarget selects the control-plane endpoint according to the supplied options,
// environment overrides, and cached descriptors.
func ResolveTarget(ctx context.Context, opts Options) (Target, error) {
	var target Target

	desc, haveDescriptor, err := resolveDescriptor(opts.ClusterID)
	if err != nil {
		return target, err
	}

	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv(controlPlaneURLEnv))
	}
	if endpoint == "" && haveDescriptor {
		endpoint, err = BaseURLFromDescriptor(desc)
		if err != nil {
			return target, err
		}
	}
	if endpoint == "" {
		return target, errors.New("control plane endpoint not configured; set PLOY_CONTROL_PLANE_URL or add a cluster descriptor")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return target, fmt.Errorf("parse control plane url: %w", err)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	if parsed.Host == "" {
		return target, fmt.Errorf("control plane url missing host: %s", endpoint)
	}

	if !haveDescriptor && strings.TrimSpace(opts.ClusterID) != "" {
		if seeded, ok, seedErr := seedDescriptorFromConfig(ctx, parsed, strings.TrimSpace(opts.ClusterID)); seedErr == nil && ok {
			desc = seeded
			haveDescriptor = true
		} else if seedErr != nil {
			return target, seedErr
		}
	}

	target.BaseURL = parsed
	if haveDescriptor {
		d := desc
		target.Descriptor = &d
		target.ClusterID = d.ClusterID
	} else {
		target.ClusterID = strings.TrimSpace(opts.ClusterID)
	}
	return target, nil
}

// ResolveHTTP returns a configured HTTP client plus the base URL for control-plane requests.
func ResolveHTTP(ctx context.Context, opts Options) (*url.URL, *http.Client, error) {
	target, err := ResolveTarget(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	client, err := newHTTPClient(target)
	if err != nil {
		return nil, nil, err
	}
	return target.BaseURL, client, nil
}

// BaseURLFromDescriptor derives the HTTPS endpoint from the provided descriptor metadata.
func BaseURLFromDescriptor(desc config.Descriptor) (string, error) {
	// Prefer explicit HTTPS endpoints if provided.
	if len(desc.APIEndpoints) > 0 {
		trimmed := strings.TrimSpace(desc.APIEndpoints[0])
		if trimmed == "" {
			return "", errors.New("descriptor api_endpoints contains empty entry")
		}
		if !strings.Contains(trimmed, "://") {
			trimmed = "https://" + trimmed
		}
		return trimmed, nil
	}
	addr := strings.TrimSpace(desc.Address)
	if addr == "" {
		return "", errors.New("cluster descriptor missing address; re-run 'ploy cluster add'")
	}
	if strings.Contains(addr, "://") {
		parsed, err := url.Parse(addr)
		if err != nil {
			return "", fmt.Errorf("parse descriptor address: %w", err)
		}
		if parsed.Scheme == "" {
			scheme := strings.TrimSpace(desc.Scheme)
			if scheme == "" {
				scheme = "https"
			}
			parsed.Scheme = scheme
		}
		if parsed.Port() == "" {
			parsed.Host = net.JoinHostPort(parsed.Hostname(), strconv.Itoa(portForScheme(parsed.Scheme)))
		}
		return parsed.String(), nil
	}

	scheme := strings.TrimSpace(desc.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if _, err := strconv.Atoi(port); err != nil {
			return "", fmt.Errorf("invalid control plane port %q", port)
		}
		return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, port)), nil
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(addr, strconv.Itoa(DefaultPort))), nil
}

func newHTTPClient(target Target) (*http.Client, error) {
    transport := http.DefaultTransport.(*http.Transport).Clone()
    transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
    if target.Descriptor != nil {
        pem := strings.TrimSpace(target.Descriptor.CABundle)
        if pem != "" {
            pool := x509.NewCertPool()
            if !pool.AppendCertsFromPEM([]byte(pem)) {
                return nil, errors.New("control plane descriptor CA bundle invalid")
            }
            transport.TLSClientConfig.RootCAs = pool
        }
    }
    // Prefer explicit ServerName when provided.
    if target.Descriptor != nil && strings.TrimSpace(target.Descriptor.APIServerName) != "" {
        transport.TLSClientConfig.ServerName = strings.TrimSpace(target.Descriptor.APIServerName)
    } else if isLoopbackHost(target.BaseURL.Hostname()) && target.Descriptor != nil {
        // Guard: loopback host while reaching a remote certificate
        host := strings.TrimSpace(target.Descriptor.Address)
        if host != "" {
            transport.TLSClientConfig.ServerName = host
        }
    }
    // SSH tunnels have been removed; all control-plane traffic is direct HTTPS.
    client := &http.Client{Timeout: defaultHTTPTimeout, Transport: transport}
	// Multi-endpoint failover transport: if descriptor lists multiple endpoints, try them in order
	// on network error or 502/503/504.
	if target.Descriptor != nil && len(target.Descriptor.APIEndpoints) > 0 {
		eps := make([]*url.URL, 0, len(target.Descriptor.APIEndpoints))
		for _, raw := range target.Descriptor.APIEndpoints {
			if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Host != "" {
				if u.Scheme == "" {
					u.Scheme = target.BaseURL.Scheme
				}
				eps = append(eps, u)
			}
		}
		if len(eps) > 0 {
			client.Transport = &multiEndpointTransport{endpoints: eps, base: client.Transport, retryStatuses: map[int]struct{}{502: {}, 503: {}, 504: {}}, serverName: transport.TLSClientConfig.ServerName}
			// pin BaseURL to first endpoint to keep command path composition consistent
			target.BaseURL = eps[0]
		}
	}
    return client, nil
}

type multiEndpointTransport struct {
    endpoints     []*url.URL
    base          http.RoundTripper
    retryStatuses map[int]struct{}
    serverName    string
}

func (m *multiEndpointTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error
	for i, ep := range m.endpoints {
		clone := req.Clone(req.Context())
		u := *clone.URL
		u.Scheme = ep.Scheme
		u.Host = ep.Host
		clone.URL = &u
		// Attempt
		resp, err := m.base.RoundTrip(clone)
		if err != nil {
			lastErr = err
			continue
		}
		if _, retry := m.retryStatuses[resp.StatusCode]; retry && i+1 < len(m.endpoints) {
			// Drain and try next endpoint
			_ = resp.Body.Close()
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func descriptorHost(address string, base *url.URL) (string, error) {
    addr := strings.TrimSpace(address)
    if addr == "" && base != nil {
        return base.Hostname(), nil
    }
	if addr == "" {
		return "", errors.New("cluster descriptor missing address")
	}
	if strings.Contains(addr, "://") {
		parsed, err := url.Parse(addr)
		if err != nil {
			return "", fmt.Errorf("parse descriptor address: %w", err)
		}
		return parsed.Hostname(), nil
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host, nil
	}
	return addr, nil
}

func apiPort(base *url.URL) int {
	if base == nil {
		return DefaultPort
	}
	if port := base.Port(); port != "" {
		if value, err := strconv.Atoi(port); err == nil {
			return value
		}
	}
	return portForScheme(base.Scheme)
}

func portForScheme(scheme string) int {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "http":
		return 80
	default:
		return DefaultPort
	}
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

func resolveDescriptor(clusterID string) (config.Descriptor, bool, error) {
	trimmed := strings.TrimSpace(clusterID)
	if trimmed != "" {
		desc, err := config.LoadDescriptor(trimmed)
		if err == nil {
			return desc, true, nil
		}
		if isDescriptorMissing(err) {
			return config.Descriptor{}, false, nil
		}
		return config.Descriptor{}, false, err
	}
	descs, err := config.ListDescriptors()
	if err != nil {
		return config.Descriptor{}, false, err
	}
	if len(descs) == 0 {
		return config.Descriptor{}, false, nil
	}
	for _, desc := range descs {
		if desc.Default {
			return desc, true, nil
		}
	}
	if len(descs) == 1 {
		return descs[0], true, nil
	}
	return config.Descriptor{}, false, errors.New("multiple clusters cached; specify --cluster-id")
}

func isDescriptorMissing(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}

func seedDescriptorFromConfig(ctx context.Context, base *url.URL, clusterID string) (config.Descriptor, bool, error) {
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return config.Descriptor{}, false, nil
	}
	client := &http.Client{Timeout: defaultHTTPTimeout}
	desc, err := fetchDescriptorFromControlPlane(ctx, client, base, trimmed)
	if err != nil {
		return config.Descriptor{}, false, err
	}
	if desc.ClusterID == "" {
		return config.Descriptor{}, false, nil
	}
	if _, err := config.SaveDescriptor(desc); err != nil {
		return config.Descriptor{}, false, err
	}
	existing, err := config.ListDescriptors()
	if err == nil && len(existing) <= 1 {
		_ = config.SetDefault(desc.ClusterID)
	}
	return desc, true, nil
}

func fetchDescriptorFromControlPlane(ctx context.Context, client *http.Client, base *url.URL, clusterID string) (config.Descriptor, error) {
	if client == nil {
		return config.Descriptor{}, errors.New("control plane client required")
	}
	endpoint := *base
	q := endpoint.Query()
	q.Set("cluster_id", clusterID)
	endpoint.RawQuery = q.Encode()
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/v1/config"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return config.Descriptor{}, fmt.Errorf("build cluster config request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return config.Descriptor{}, fmt.Errorf("fetch cluster config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return config.Descriptor{}, fmt.Errorf("fetch cluster config: %s", resp.Status)
	}
	var payload struct {
		ClusterID string         `json:"cluster_id"`
		Config    map[string]any `json:"config"`
		Revision  json.Number    `json:"revision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return config.Descriptor{}, fmt.Errorf("decode cluster config: %w", err)
	}
	return descriptorFromDiscovery(payload.Config, clusterID), nil
}

func descriptorFromDiscovery(doc map[string]any, requested string) config.Descriptor {
	lookup := strings.TrimSpace(requested)
	discovery, ok := doc["discovery"].(map[string]any)
	if !ok {
		return config.Descriptor{}
	}
	defaultID := strings.TrimSpace(asString(discovery["default_descriptor"]))
	entries, ok := discovery["descriptors"].([]any)
	if !ok || len(entries) == 0 {
		return config.Descriptor{}
	}
	var chosen map[string]any
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		cid := strings.TrimSpace(asString(entry["cluster_id"]))
		if cid == "" {
			continue
		}
		if lookup != "" && cid == lookup {
			chosen = entry
			break
		}
		if lookup == "" && cid == defaultID {
			chosen = entry
			break
		}
		if chosen == nil && defaultID == "" {
			chosen = entry
		}
	}
	if chosen == nil {
		chosen, _ = entries[0].(map[string]any)
	}
	if chosen == nil {
		return config.Descriptor{}
	}
	clusterID := strings.TrimSpace(asString(chosen["cluster_id"]))
	if clusterID == "" {
		return config.Descriptor{}
	}
	address := strings.TrimSpace(asString(chosen["address"]))
	api := strings.TrimSpace(asString(chosen["api_endpoint"]))
	if address == "" && api != "" {
		if parsed, err := url.Parse(api); err == nil {
			address = parsed.Hostname()
		}
	}
	scheme := ""
	if api != "" {
		if parsed, err := url.Parse(api); err == nil {
			scheme = parsed.Scheme
			if address == "" {
				address = parsed.Host
			}
		}
	}
	if address == "" {
		return config.Descriptor{}
	}
	labels := map[string]string{}
	if rawLabels, ok := chosen["labels"].(map[string]any); ok {
		for k, v := range rawLabels {
			labels[strings.TrimSpace(k)] = strings.TrimSpace(asString(v))
		}
	}
	return config.Descriptor{
		ClusterID:       clusterID,
		Address:         address,
		Labels:          labels,
		Scheme:          scheme,
		CABundle:        strings.TrimSpace(asString(chosen["ca_bundle"])),
		SSHIdentityPath: strings.TrimSpace(asString(chosen["ssh_identity_path"])),
	}
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func defaultIdentityPath() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_SSH_IDENTITY")); value != "" {
		return expandPath(value)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "id_rsa")
}

func defaultSSHUser() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_SSH_USER")); value != "" {
		return value
	}
	return "ploy"
}
