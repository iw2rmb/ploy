package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/cmd/ploy/config"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/deploy"
)

const (
	defaultWorkerJoinTimeout = 20 * time.Second
)

type nodeAddRequest struct {
	ClusterID string                     `json:"cluster_id"`
	Address   string                     `json:"address"`
	Labels    map[string]string          `json:"labels,omitempty"`
	Probes    []deploy.WorkerHealthProbe `json:"probes,omitempty"`
	DryRun    bool                       `json:"dry_run,omitempty"`
}

type nodeAddResponse struct {
	WorkerID    string                       `json:"worker_id"`
	Certificate deploy.LeafCertificate       `json:"certificate"`
	DryRun      bool                         `json:"dry_run"`
	Health      []registry.WorkerProbeResult `json:"health"`
}

func handleNode(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printNodeUsage(stderr)
		return errors.New("node subcommand required")
	}
	switch args[0] {
	case "add":
		return runNodeAdd(args[1:], stderr)
	default:
		printNodeUsage(stderr)
		return fmt.Errorf("unknown node subcommand %q", args[0])
	}
}

func runNodeAdd(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("node add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		address stringValue
	)
	labels := make(labelMap)
	probes := make(probeList, 0)

	fs.Var(&address, "address", "Worker address (host or IP)")
	fs.Var(&labels, "label", "Apply a label (key=value). May be repeated.")
	fs.Var(&probes, "health-probe", "Register a health probe in the form name=url. May be repeated.")
	dryRun := fs.Bool("dry-run", false, "Preview onboarding without registering the node")

	if err := fs.Parse(args); err != nil {
		printNodeAddUsage(stderr)
		return err
	}

	if fs.NArg() > 1 {
		printNodeAddUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !address.set && fs.NArg() == 1 {
		address.value = strings.TrimSpace(fs.Arg(0))
		address.set = true
	}

	if !address.set || strings.TrimSpace(address.value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("worker address is required")
	}

	desc, err := resolveActiveClusterDescriptor()
	if err != nil {
		return err
	}

	payload := nodeAddRequest{
		ClusterID: desc.ID,
		Address:   strings.TrimSpace(address.value),
		Labels:    map[string]string(labels),
		Probes:    []deploy.WorkerHealthProbe(probes),
		DryRun:    *dryRun,
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultWorkerJoinTimeout)
	defer cancel()

	url := buildAdminURL(payload.Address)
	result, err := registerNode(ctx, url, payload)
	if err != nil {
		return err
	}

	if result.DryRun {
		if err := writef(stderr, "[DRY RUN] Worker %s would be added to cluster %s\n", result.WorkerID, desc.ID); err != nil {
			return err
		}
	} else {
		if err := writef(stderr, "Worker %s joined cluster %s\n", result.WorkerID, desc.ID); err != nil {
			return err
		}
		if result.Certificate.Version != "" {
			if err := writef(stderr, "Certificate version: %s (parent %s)\n", result.Certificate.Version, result.Certificate.ParentVersion); err != nil {
				return err
			}
		}
	}
	if len(result.Health) > 0 {
		label := "Health probes:"
		if result.DryRun {
			label = "Health probe preview:"
		}
		if err := writef(stderr, "%s\n", label); err != nil {
			return err
		}
		for _, probe := range result.Health {
			state := "pass"
			if !probe.Passed {
				state = "FAIL"
			}
			if err := writef(stderr, "  - %s (%s): %s", probe.Name, probe.Endpoint, state); err != nil {
				return err
			}
			if probe.StatusCode != 0 {
				if err := writef(stderr, " status=%d", probe.StatusCode); err != nil {
					return err
				}
			}
			if probe.Message != "" {
				if err := writef(stderr, " (%s)", probe.Message); err != nil {
					return err
				}
			}
			if err := writef(stderr, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildAdminURL(address string) string {
	if base := strings.TrimSpace(os.Getenv("PLOYD_ADMIN_ENDPOINT")); base != "" {
		return strings.TrimRight(base, "/") + "/admin/v1/nodes"
	}
	scheme := strings.TrimSpace(os.Getenv("PLOYD_ADMIN_SCHEME"))
	if scheme == "" {
		scheme = "http"
	}
	port := strings.TrimSpace(os.Getenv("PLOYD_ADMIN_PORT"))
	if port == "" {
		port = "8443"
	}
	host := strings.TrimSpace(address)
	if host == "" {
		host = "localhost"
	}
	if strings.Contains(host, "://") {
		return strings.TrimRight(host, "/") + "/admin/v1/nodes"
	}
	if strings.Count(host, ":") == 1 && !strings.HasSuffix(host, "]") {
		return fmt.Sprintf("%s://%s/admin/v1/nodes", scheme, host)
	}
	return fmt.Sprintf("%s://%s:%s/admin/v1/nodes", scheme, host, port)
}

func registerNode(ctx context.Context, url string, payload nodeAddRequest) (nodeAddResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nodeAddResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nodeAddResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nodeAddResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		if len(msg) == 0 {
			return nodeAddResponse{}, fmt.Errorf("ployd responded with %s", resp.Status)
		}
		return nodeAddResponse{}, fmt.Errorf("ployd: %s", strings.TrimSpace(string(msg)))
	}
	var out nodeAddResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nodeAddResponse{}, err
	}
	return out, nil

}

type labelMap map[string]string

func (m labelMap) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("label cannot be empty")
	}
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid label %q, expected key=value", value)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return fmt.Errorf("invalid label %q, key required", value)
	}
	val := strings.TrimSpace(parts[1])
	if m == nil {
		return errors.New("label map not initialised")
	}
	m[key] = val
	return nil
}

func (m labelMap) String() string {
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(pairs, ",")
}

type probeList []deploy.WorkerHealthProbe

func (p *probeList) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("health probe cannot be empty")
	}
	name := ""
	endpoint := trimmed
	if strings.Contains(trimmed, "=") {
		parts := strings.SplitN(trimmed, "=", 2)
		name = strings.TrimSpace(parts[0])
		endpoint = strings.TrimSpace(parts[1])
	}
	if endpoint == "" {
		return fmt.Errorf("invalid health probe %q, endpoint required", value)
	}
	if name == "" {
		name = fmt.Sprintf("probe-%d", len(*p)+1)
	}
	*p = append(*p, deploy.WorkerHealthProbe{
		Name:     name,
		Endpoint: endpoint,
	})
	return nil
}

func (p *probeList) String() string {
	if p == nil {
		return ""
	}
	items := make([]string, 0, len(*p))
	for _, probe := range *p {
		items = append(items, fmt.Sprintf("%s=%s", probe.Name, probe.Endpoint))
	}
	return strings.Join(items, ",")
}

func printNodeUsage(w io.Writer) {
	printCommandUsage(w, "node")
}

func printNodeAddUsage(w io.Writer) {
	printCommandUsage(w, "node", "add")
}

func resolveActiveClusterDescriptor() (config.Descriptor, error) {
	descs, err := config.ListDescriptors()
	if err != nil {
		return config.Descriptor{}, err
	}
	if len(descs) == 0 {
		return config.Descriptor{}, errors.New("no clusters cached locally; run 'ploy deploy bootstrap' first")
	}
	for _, desc := range descs {
		if desc.Default {
			return desc, nil
		}
	}
	if len(descs) == 1 {
		return descs[0], nil
	}
	return config.Descriptor{}, errors.New("multiple clusters cached; set a default in ~/.config/ploy/clusters before onboarding nodes")
}
