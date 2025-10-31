package main

import (
    "context"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "io"
    "os"
    "net/http"
    "net/url"
    "sort"
    "strings"

    "github.com/iw2rmb/ploy/internal/cli/config"
    "github.com/iw2rmb/ploy/internal/cli/controlplane"
)

func handleCluster(args []string, w io.Writer) error {
	if len(args) == 0 {
		printClusterUsage(w)
		return errors.New("cluster subcommand required")
	}
	switch args[0] {
	case "list":
		return handleClusterList(w)
	case "add":
		return handleClusterAdd(args[1:], w)
	case "https":
		return handleClusterHTTPS(args[1:], w)
	case "connect":
		return handleClusterConnect(args[1:], w)
	case "cert":
		return handleClusterCert(args[1:], w)
	default:
		printClusterUsage(w)
		return fmt.Errorf("unknown cluster subcommand %q", args[0])
	}
}

func handleClusterList(w io.Writer) error {
	descs, err := config.ListDescriptors()
	if err != nil {
		return err
	}
	if len(descs) == 0 {
		_, _ = fmt.Fprintln(w, "No clusters cached. Run 'ploy cluster add' to add one.")
		return nil
	}
	_, _ = fmt.Fprintf(w, "Clusters (%d):\n", len(descs))
	for _, desc := range descs {
		label := desc.ClusterID
		if desc.Default {
			label += " (default)"
		}
		_, _ = fmt.Fprintf(w, "  - %s  address=%s", label, desc.Address)
		if trimmed := strings.TrimSpace(desc.SSHIdentityPath); trimmed != "" {
			_, _ = fmt.Fprintf(w, "  identity=%s", trimmed)
		}
		if formatted := formatDescriptorLabels(desc.Labels); formatted != "" {
			_, _ = fmt.Fprintf(w, "  labels=%s", formatted)
		}
		_, _ = fmt.Fprintln(w)
	}
	return nil
}

func handleClusterConnect(args []string, w io.Writer) error {
    printClusterUsage(w)
    return errors.New("cluster connect not yet implemented")
}

// handleClusterHTTPS patches a cached descriptor with HTTPS endpoints, SNI server name,
// registry host, and an optional CA bundle, then saves it.
func handleClusterHTTPS(args []string, w io.Writer) error {
    fs := flag.NewFlagSet("cluster https", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    var clusterID stringValue
    var apiServerName string
    var registryHost string
    var caFile string
    var disableSSH bool
    var endpoints multiString
    fs.Var(&clusterID, "cluster-id", "Cluster identifier to update (default: current)")
    fs.Var(&endpoints, "api-endpoint", "Control-plane HTTPS endpoint (repeatable)")
    fs.StringVar(&apiServerName, "api-server-name", "", "TLS ServerName (SNI) to verify, e.g. api.<cluster-id>.ploy")
    fs.StringVar(&registryHost, "registry-host", "", "(deprecated) Registry host previously used by nodes")
    fs.StringVar(&caFile, "ca-file", "", "Path to CA bundle PEM to trust")
    fs.BoolVar(&disableSSH, "disable-ssh", false, "Disable SSH tunnels and prefer HTTPS endpoints only")
    if err := fs.Parse(args); err != nil { printClusterUsage(w); return err }
    if fs.NArg() > 0 { printClusterUsage(w); return fmt.Errorf("unexpected args: %s", strings.Join(fs.Args(), " ")) }

    desc, err := loadClusterDescriptor(clusterID.value)
    if err != nil { return err }
    if len(endpoints.values) > 0 {
        desc.APIEndpoints = endpoints.values
        // Prefer first endpoint as Address for compatibility
        desc.Address = strings.TrimSpace(endpoints.values[0])
    }
    if strings.TrimSpace(apiServerName) != "" { desc.APIServerName = strings.TrimSpace(apiServerName) }
    if strings.TrimSpace(registryHost) != "" { desc.RegistryHost = strings.TrimSpace(registryHost) }
    if disableSSH { desc.DisableSSH = true }
    if strings.TrimSpace(caFile) != "" {
        data, err := os.ReadFile(strings.TrimSpace(caFile))
        if err != nil { return fmt.Errorf("read ca file: %w", err) }
        desc.CABundle = string(data)
    }
    if _, err := config.SaveDescriptor(desc); err != nil { return err }
    _, _ = fmt.Fprintf(w, "Updated descriptor %s\n", desc.ClusterID)
    if len(desc.APIEndpoints) > 0 {
        _, _ = fmt.Fprintf(w, "  api_endpoints: %s\n", strings.Join(desc.APIEndpoints, ", "))
    }
    if desc.APIServerName != "" { _, _ = fmt.Fprintf(w, "  api_server_name: %s\n", desc.APIServerName) }
    if desc.RegistryHost != "" { _, _ = fmt.Fprintf(w, "  registry_host: %s\n", desc.RegistryHost) }
    if desc.DisableSSH { _, _ = fmt.Fprintln(w, "  disable_ssh: true") }
    if strings.TrimSpace(desc.CABundle) != "" { _, _ = fmt.Fprintln(w, "  ca_bundle: (set)") }
    return nil
}

type multiString struct{ values []string }
func (m *multiString) String() string { return strings.Join(m.values, ",") }
func (m *multiString) Set(v string) error {
    v = strings.TrimSpace(v)
    if v == "" { return nil }
    m.values = append(m.values, v)
    return nil
}

func formatDescriptorLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(values, ",")
}

func handleClusterCert(args []string, w io.Writer) error {
	if len(args) == 0 {
		printClusterUsage(w)
		return errors.New("cluster cert subcommand required")
	}
	switch args[0] {
	case "status":
		return handleClusterCertStatus(args[1:], w)
	default:
		printClusterUsage(w)
		return fmt.Errorf("unknown cluster cert subcommand %q", args[0])
	}
}

func handleClusterCertStatus(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("cluster cert status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var clusterID stringValue
	fs.Var(&clusterID, "cluster-id", "Cluster identifier to inspect (default: current)")
	if err := fs.Parse(args); err != nil {
		printClusterUsage(w)
		return err
	}
	if fs.NArg() > 0 {
		printClusterUsage(w)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	desc, err := loadClusterDescriptor(clusterID.value)
	if err != nil {
		return err
	}
	baseURL, err := controlplane.BaseURLFromDescriptor(desc)
	if err != nil {
		return err
	}

	factory := clusterHTTPClientFactory
	if factory == nil {
		factory = newDescriptorHTTPClient
	}
	client, cleanup, err := factory(desc)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	status, err := fetchClusterCAStatus(context.Background(), client, baseURL, desc.ClusterID)
	if err != nil {
		return err
	}
	return renderClusterCAStatus(w, status)
}

func loadClusterDescriptor(clusterID string) (config.Descriptor, error) {
	trimmed := strings.TrimSpace(clusterID)
	if trimmed != "" {
		return config.LoadDescriptor(trimmed)
	}
	descs, err := config.ListDescriptors()
	if err != nil {
		return config.Descriptor{}, err
	}
	if len(descs) == 0 {
		return config.Descriptor{}, errors.New("no clusters cached; run 'ploy cluster add' first")
	}
	for _, desc := range descs {
		if desc.Default {
			return desc, nil
		}
	}
	if len(descs) == 1 {
		return descs[0], nil
	}
	return config.Descriptor{}, errors.New("multiple clusters cached; specify --cluster-id")
}

func fetchClusterCAStatus(ctx context.Context, client *http.Client, baseURL, clusterID string) (caStatusResponse, error) {
	var status caStatusResponse
	if client == nil {
		return status, errors.New("cluster HTTP client not configured")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/security/ca?cluster_id=" + url.QueryEscape(strings.TrimSpace(clusterID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return status, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return status, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return status, fmt.Errorf("cluster %s certificate authority not bootstrapped", clusterID)
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		if len(data) == 0 {
			data = []byte(resp.Status)
		}
		return status, fmt.Errorf("fetch CA status: %s", strings.TrimSpace(string(data)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return status, fmt.Errorf("decode CA status: %w", err)
	}
	return status, nil
}

func renderClusterCAStatus(w io.Writer, status caStatusResponse) error {
	if strings.TrimSpace(status.ClusterID) == "" {
		return errors.New("invalid CA status payload")
	}
	_, err := fmt.Fprintf(w, "Cluster %s certificate authority\n", status.ClusterID)
	if err != nil {
		return err
	}
	if v := strings.TrimSpace(status.CurrentCA.Version); v != "" {
		if _, err := fmt.Fprintf(w, "  Version: %s\n", v); err != nil {
			return err
		}
	}
	if serial := strings.TrimSpace(status.CurrentCA.SerialNumber); serial != "" {
		if _, err := fmt.Fprintf(w, "  Serial: %s\n", serial); err != nil {
			return err
		}
	}
	if issued := strings.TrimSpace(status.CurrentCA.IssuedAt); issued != "" {
		if _, err := fmt.Fprintf(w, "  Issued: %s\n", issued); err != nil {
			return err
		}
	}
	if expires := strings.TrimSpace(status.CurrentCA.ExpiresAt); expires != "" {
		if _, err := fmt.Fprintf(w, "  Expires: %s\n", expires); err != nil {
			return err
		}
	}
	if status.Workers.Total > 0 {
		if _, err := fmt.Fprintf(w, "  Workers: %d\n", status.Workers.Total); err != nil {
			return err
		}
	}
	if status.ControlPlane.Total > 0 {
		if _, err := fmt.Fprintf(w, "  Control-plane nodes: %d\n", status.ControlPlane.Total); err != nil {
			return err
		}
	}
	if hash := strings.TrimSpace(status.TrustBundleHash); hash != "" {
		if _, err := fmt.Fprintf(w, "  Trust bundle hash: %s\n", hash); err != nil {
			return err
		}
	}
	return nil
}

type caStatusResponse struct {
	ClusterID string `json:"cluster_id"`
	CurrentCA struct {
		Version      string `json:"version"`
		IssuedAt     string `json:"issued_at"`
		ExpiresAt    string `json:"expires_at"`
		SerialNumber string `json:"serial_number"`
	} `json:"current_ca"`
	Workers struct {
		Total int `json:"total"`
	} `json:"workers"`
	ControlPlane struct {
		Total int `json:"total"`
	} `json:"control_plane"`
	TrustBundleHash string `json:"trust_bundle_hash"`
}
