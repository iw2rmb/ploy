package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/deploy"
)

const (
	defaultWorkerJoinTimeout = 20 * time.Second
)

type nodeJoinRequest struct {
	ClusterID string            `json:"cluster_id"`
	WorkerID  string            `json:"worker_id,omitempty"`
	Address   string            `json:"address"`
	Labels    map[string]string `json:"labels,omitempty"`
	Probes    []nodeJoinProbe   `json:"probes,omitempty"`
	DryRun    bool              `json:"dry_run,omitempty"`
}

type nodeJoinProbe struct {
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	ExpectStatus int    `json:"expect_status"`
}

type nodeJoinResponse struct {
	WorkerID    string                       `json:"worker_id"`
	Certificate deploy.LeafCertificate       `json:"certificate"`
	Health      []registry.WorkerProbeResult `json:"health"`
	DryRun      bool                         `json:"dry_run"`
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
		address  stringValue
		user     stringValue
		identity stringValue
		ploydBin stringValue
		sshPort  intValue
	)
	labels := make(labelMap)
	probes := make(probeList, 0)

	fs.Var(&address, "address", "Worker address (host or IP)")
	fs.Var(&labels, "label", "Apply a label (key=value). May be repeated.")
	fs.Var(&probes, "health-probe", "Register a health probe in the form name=url. May be repeated.")
	fs.Var(&user, "user", "SSH username (default: root)")
	fs.Var(&identity, "identity", "SSH identity file (default: ~/.ssh/id_rsa)")
	fs.Var(&sshPort, "ssh-port", "SSH port (default: 22)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd binary uploaded during provisioning (default: alongside the CLI)")
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

	baseURL, err := controlPlaneBaseURL(desc)
	if err != nil {
		return err
	}

	workerAddress := strings.TrimSpace(address.value)

	identityPath := strings.TrimSpace(identity.value)
	if identity.set {
		identityPath = deploycli.ExpandPath(identityPath)
	} else {
		identityPath = deploycli.DefaultIdentityPath()
	}

	var ploydPath string
	if !*dryRun {
		if ploydBin.set {
			ploydPath = deploycli.ExpandPath(strings.TrimSpace(ploydBin.value))
		} else {
			ploydPath, err = deploycli.DefaultPloydBinaryPath("")
			if err != nil {
				return err
			}
		}
	}

	if !*dryRun {
		provEnv := map[string]string{
			"PLOY_CONTROL_PLANE_ENDPOINT": baseURL,
		}
		provOpts := deploy.ProvisionOptions{
			Host:            workerAddress,
			Address:         workerAddress,
			User:            strings.TrimSpace(user.value),
			Port:            sshPort.value,
			IdentityFile:    identityPath,
			PloydBinaryPath: ploydPath,
			Stdout:          stderr,
			Stderr:          stderr,
			Mode:            deploy.ProvisionModeWorker,
			ScriptEnv:       provEnv,
			ServiceChecks:   []string{"ployd"},
		}
		if err := deploy.ProvisionHost(context.Background(), provOpts); err != nil {
			return err
		}
	}

	probesPayload := make([]nodeJoinProbe, 0, len(probes))
	for _, probe := range probes {
		probesPayload = append(probesPayload, nodeJoinProbe{
			Name:         strings.TrimSpace(probe.Name),
			Endpoint:     strings.TrimSpace(probe.Endpoint),
			ExpectStatus: probe.ExpectStatus,
		})
	}

	request := nodeJoinRequest{
		ClusterID: desc.ID,
		Address:   workerAddress,
		Labels:    map[string]string(labels),
		Probes:    probesPayload,
		DryRun:    *dryRun,
	}

	client, err := newControlPlaneClient(baseURL, desc, defaultWorkerJoinTimeout)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultWorkerJoinTimeout)
	defer cancel()

	result, err := registerWorker(ctx, client, baseURL, request)
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

func controlPlaneBaseURL(desc config.Descriptor) (string, error) {
	if base := strings.TrimSpace(desc.ControlPlaneURL); base != "" {
		return strings.TrimRight(base, "/"), nil
	}
	if base := strings.TrimSpace(desc.BeaconURL); base != "" {
		return strings.TrimRight(base, "/"), nil
	}
	return "", errors.New("control-plane URL missing; rerun 'ploy deploy bootstrap'")
}

func newControlPlaneClient(baseURL string, desc config.Descriptor, timeout time.Duration) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.HasPrefix(baseURL, "https://") {
		caPath := strings.TrimSpace(desc.CABundlePath)
		if caPath == "" {
			return nil, errors.New("cluster CA bundle path missing; rerun 'ploy deploy bootstrap'")
		}
		data, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read cluster CA bundle: %w", err)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, errors.New("cluster CA bundle invalid")
		}
		transport.TLSClientConfig = &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

func registerWorker(ctx context.Context, client *http.Client, baseURL string, payload nodeJoinRequest) (nodeJoinResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nodeJoinResponse{}, err
	}
	url := strings.TrimRight(baseURL, "/") + "/v2/nodes"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nodeJoinResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nodeJoinResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		if len(msg) == 0 {
			msg = []byte(resp.Status)
		}
		return nodeJoinResponse{}, fmt.Errorf("worker registration failed: %s", strings.TrimSpace(string(msg)))
	}
	var out nodeJoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nodeJoinResponse{}, err
	}
	if out.WorkerID == "" {
		out.WorkerID = payload.WorkerID
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
