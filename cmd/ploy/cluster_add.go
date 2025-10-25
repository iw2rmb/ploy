package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

const defaultControlPlanePort = 8443

var (
	clusterBootstrapRunner                               = deploy.RunBootstrap
	clusterProvisionHost                                 = deploy.ProvisionHost
	clusterWorkerRegister                                = registerWorker
	clusterHTTPClientFactory descriptorHTTPClientFactory = newDescriptorHTTPClient
)

type descriptorHTTPClientFactory func(config.Descriptor) (*http.Client, func(), error)

func handleClusterAdd(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("cluster add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		address   stringValue
		clusterID stringValue
		identity  stringValue
		userFlag  stringValue
		control   stringValue
		ploydBin  stringValue
		sshPort   intValue
	)
	labels := make(labelMap)
	probes := make(probeList, 0)
	dryRun := fs.Bool("dry-run", false, "Preview worker onboarding without registering the node")

	fs.Var(&address, "address", "Target host or IP address")
	fs.Var(&clusterID, "cluster-id", "Existing cluster identifier to join as a worker")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&control, "control-plane-url", "Control-plane endpoint recorded during bootstrap (default: http://127.0.0.1:9094)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd binary uploaded during provisioning (default: alongside the CLI)")
	fs.Var(&sshPort, "ssh-port", "SSH port for worker provisioning (default: 22)")
	fs.Var(&labels, "label", "Apply a worker label key=value (worker mode only). May be repeated.")
	fs.Var(&probes, "health-probe", "Register a worker health probe as name=url (worker mode only). May be repeated.")

	if err := fs.Parse(args); err != nil {
		printClusterAddUsage(stderr)
		return err
	}
	if fs.NArg() > 0 {
		printClusterAddUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		printClusterAddUsage(stderr)
		return errors.New("address is required")
	}
	isWorker := clusterID.set && strings.TrimSpace(clusterID.value) != ""
	if !isWorker {
		if len(labels) > 0 || len(probes) > 0 || *dryRun || sshPort.set {
			printClusterAddUsage(stderr)
			return errors.New("worker-only flags require --cluster-id")
		}
		return runClusterBootstrap(address.value, userFlag, identity, control, ploydBin, stderr)
	}
	if control.set {
		printClusterAddUsage(stderr)
		return errors.New("--control-plane-url is only valid when bootstrapping the first node")
	}
	workerIdentity, err := resolveIdentityPath(identity)
	if err != nil {
		return err
	}
	workerPloyd, err := resolvePloydBinaryPath(ploydBin)
	if err != nil {
		return err
	}
	workerCfg := workerProvisionConfig{
		ClusterID:     clusterID.value,
		WorkerAddress: address.value,
		User:          userFlag.value,
		IdentityFile:  workerIdentity,
		PloydBinary:   workerPloyd,
		SSHPort:       sshPort.value,
		Labels:        cloneLabelMap(labels),
		Probes:        append(make(probeList, 0, len(probes)), probes...),
		DryRun:        *dryRun,
	}
	return runClusterWorkerAdd(workerCfg, stderr)
}

func printClusterAddUsage(w io.Writer) {
	printCommandUsage(w, "cluster", "add")
}

func runClusterBootstrap(address string, userFlag, identity, control, ploydBin stringValue, stderr io.Writer) error {
	addr := strings.TrimSpace(address)
	if addr == "" {
		return errors.New("address is required")
	}
	cfg := deploycli.BootstrapConfig{
		Address:       addr,
		Stdout:        stderr,
		Stderr:        stderr,
		Stdin:         os.Stdin,
		WorkstationOS: runtime.GOOS,
	}
	if userFlag.set {
		cfg.User = strings.TrimSpace(userFlag.value)
	}
	if identity.set {
		cfg.IdentityFile = strings.TrimSpace(identity.value)
	}
	if control.set {
		cfg.ControlPlaneURL = strings.TrimSpace(control.value)
	}
	if ploydBin.set {
		cfg.PloydBinaryPath = strings.TrimSpace(ploydBin.value)
	}
	cmd := deploycli.BootstrapCommand{RunBootstrap: clusterBootstrapRunner}
	if err := cmd.Run(context.Background(), cfg); err != nil {
		return err
	}
	return writeClusterAddNextSteps(stderr, addr)
}

func writeClusterAddNextSteps(w io.Writer, clusterRef string) error {
	trimmed := strings.TrimSpace(clusterRef)
	if trimmed == "" || w == nil {
		return nil
	}
	desc, err := config.LoadDescriptor(trimmed)
	if err != nil {
		return nil
	}
	_, err = fmt.Fprintf(w, "Cluster %s cached. Add workers via 'ploy cluster add --cluster-id %s --address <worker-host>'.\n", desc.ClusterID, desc.ClusterID)
	return err
}

type workerProvisionConfig struct {
	ClusterID     string
	WorkerAddress string
	User          string
	IdentityFile  string
	PloydBinary   string
	SSHPort       int
	Labels        map[string]string
	Probes        []deploy.WorkerHealthProbe
	DryRun        bool
}

func runClusterWorkerAdd(cfg workerProvisionConfig, stderr io.Writer) error {
	clusterID := strings.TrimSpace(cfg.ClusterID)
	if clusterID == "" {
		return errors.New("cluster id required")
	}
	desc, err := config.LoadDescriptor(clusterID)
	if err != nil {
		return err
	}
	workerAddr := strings.TrimSpace(cfg.WorkerAddress)
	if workerAddr == "" {
		return errors.New("worker address is required")
	}
	baseURL, err := descriptorControlPlaneURL(desc)
	if err != nil {
		return err
	}
	if !cfg.DryRun {
		provOpts := deploy.ProvisionOptions{
			Host:            workerAddr,
			Address:         workerAddr,
			User:            strings.TrimSpace(cfg.User),
			Port:            cfg.SSHPort,
			IdentityFile:    cfg.IdentityFile,
			PloydBinaryPath: cfg.PloydBinary,
			Stdout:          stderr,
			Stderr:          stderr,
			Mode:            deploy.ProvisionModeWorker,
			ScriptEnv: map[string]string{
				"PLOY_CONTROL_PLANE_ENDPOINT": baseURL,
			},
			ServiceChecks: []string{"ployd"},
		}
		if err := clusterProvisionHost(context.Background(), provOpts); err != nil {
			return err
		}
	}
	factory := clusterHTTPClientFactory
	if factory == nil {
		factory = newDescriptorHTTPClient
	}
	client, cleanup, err := factory(desc)
	if err != nil {
		return err
	}
	defer cleanup()
	payload := nodeJoinRequest{
		ClusterID: desc.ClusterID,
		Address:   workerAddr,
		Labels:    cloneLabels(cfg.Labels),
		Probes:    convertProbes(cfg.Probes),
		DryRun:    cfg.DryRun,
	}
	result, err := clusterWorkerRegister(context.Background(), client, baseURL, payload)
	if err != nil {
		return err
	}
	return renderWorkerJoinResult(stderr, desc.ClusterID, result)
}

func descriptorControlPlaneURL(desc config.Descriptor) (string, error) {
	if endpoint := strings.TrimSpace(os.Getenv("PLOYD_ADMIN_ENDPOINT")); endpoint != "" {
		return endpoint, nil
	}
	addr := strings.TrimSpace(desc.Address)
	if addr == "" {
		return "", errors.New("cluster descriptor missing address; re-run 'ploy cluster add'")
	}
	scheme := strings.TrimSpace(os.Getenv("PLOYD_ADMIN_SCHEME"))
	if scheme == "" {
		scheme = "http"
	}
	switch scheme {
	case "http", "https":
	default:
		return "", fmt.Errorf("invalid PLOYD_ADMIN_SCHEME %q", scheme)
	}
	port := defaultControlPlanePort
	if raw := strings.TrimSpace(os.Getenv("PLOYD_ADMIN_PORT")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return "", fmt.Errorf("invalid PLOYD_ADMIN_PORT %q", raw)
		}
		port = value
	}
	return fmt.Sprintf("%s://%s:%d", scheme, addr, port), nil
}

func renderWorkerJoinResult(w io.Writer, clusterID string, result nodeJoinResponse) error {
	if result.DryRun {
		if err := writef(w, "[DRY RUN] Worker %s would be added to cluster %s\n", result.WorkerID, clusterID); err != nil {
			return err
		}
	} else {
		if err := writef(w, "Worker %s joined cluster %s\n", result.WorkerID, clusterID); err != nil {
			return err
		}
		if result.Certificate.Version != "" {
			if err := writef(w, "Certificate version: %s (parent %s)\n", result.Certificate.Version, result.Certificate.ParentVersion); err != nil {
				return err
			}
		}
	}
	if len(result.Health) > 0 {
		label := "Health probes:"
		if result.DryRun {
			label = "Health probe preview:"
		}
		if err := writef(w, "%s\n", label); err != nil {
			return err
		}
		for _, probe := range result.Health {
			state := "pass"
			if !probe.Passed {
				state = "FAIL"
			}
			if err := writef(w, "  - %s (%s): %s", probe.Name, probe.Endpoint, state); err != nil {
				return err
			}
			if probe.StatusCode != 0 {
				if err := writef(w, " status=%d", probe.StatusCode); err != nil {
					return err
				}
			}
			if probe.Message != "" {
				if err := writef(w, " (%s)", probe.Message); err != nil {
					return err
				}
			}
			if err := writef(w, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveIdentityPath(value stringValue) (string, error) {
	if value.set {
		trimmed := strings.TrimSpace(value.value)
		if trimmed == "" {
			return "", errors.New("identity path cannot be empty")
		}
		return deploycli.ExpandPath(trimmed), nil
	}
	path := deploycli.DefaultIdentityPath()
	if strings.TrimSpace(path) == "" {
		return "", errors.New("unable to resolve default SSH identity; provide --identity")
	}
	return path, nil
}

func resolvePloydBinaryPath(value stringValue) (string, error) {
	if value.set {
		trimmed := strings.TrimSpace(value.value)
		if trimmed == "" {
			return "", errors.New("ployd binary path cannot be empty")
		}
		return deploycli.ExpandPath(trimmed), nil
	}
	return deploycli.DefaultPloydBinaryPath(runtime.GOOS)
}

func cloneLabelMap(labels labelMap) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for k, v := range labels {
		cloned[k] = v
	}
	return cloned
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for k, v := range labels {
		cloned[k] = v
	}
	return cloned
}

func convertProbes(probes []deploy.WorkerHealthProbe) []nodeJoinProbe {
	if len(probes) == 0 {
		return nil
	}
	out := make([]nodeJoinProbe, 0, len(probes))
	for _, probe := range probes {
		out = append(out, nodeJoinProbe{
			Name:         strings.TrimSpace(probe.Name),
			Endpoint:     strings.TrimSpace(probe.Endpoint),
			ExpectStatus: probe.ExpectStatus,
		})
	}
	return out
}

func newDescriptorHTTPClient(desc config.Descriptor) (*http.Client, func(), error) {
	addr := strings.TrimSpace(desc.Address)
	if addr == "" {
		return nil, nil, errors.New("cluster descriptor missing address; re-run 'ploy cluster add'")
	}
	identity := strings.TrimSpace(desc.SSHIdentityPath)
	if identity == "" {
		return nil, nil, errors.New("cluster descriptor missing SSH identity path")
	}
	node := sshtransport.Node{
		ID:           strings.TrimSpace(desc.ClusterID),
		Address:      addr,
		SSHPort:      22,
		APIPort:      defaultControlPlanePort,
		User:         defaultTunnelUser(),
		IdentityFile: deploycli.ExpandPath(identity),
	}
	manager, err := sshtransport.NewManager(sshtransport.Config{})
	if err != nil {
		return nil, nil, err
	}
	if err := manager.SetNodes([]sshtransport.Node{node}); err != nil {
		_ = manager.Close()
		return nil, nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.DialContext = manager.DialContext
	client := &http.Client{Timeout: defaultWorkerJoinTimeout, Transport: transport}
	cleanup := func() { _ = manager.Close() }
	return client, cleanup, nil
}

func defaultTunnelUser() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_SSH_USER")); value != "" {
		return value
	}
	return "root"
}

const defaultWorkerJoinTimeout = 20 * time.Second

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

func registerWorker(ctx context.Context, client *http.Client, baseURL string, payload nodeJoinRequest) (nodeJoinResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nodeJoinResponse{}, err
	}
	url := strings.TrimRight(baseURL, "/") + "/v1/nodes"
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
	if len(m) == 0 {
		return ""
	}
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
	*p = append(*p, deploy.WorkerHealthProbe{Name: name, Endpoint: endpoint})
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

type stringValue struct {
	value string
	set   bool
}

func (s *stringValue) Set(value string) error {
	s.value = value
	s.set = true
	return nil
}

func (s *stringValue) String() string {
	return s.value
}

type intValue struct {
	value int
	set   bool
}

func (i *intValue) Set(value string) error {
	v, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("parse int flag: %w", err)
	}
	i.value = v
	i.set = true
	return nil
}

func (i *intValue) String() string {
	if !i.set {
		return ""
	}
	return strconv.Itoa(i.value)
}

func writef(w io.Writer, format string, args ...any) error {
	if w == nil {
		return nil
	}
	_, err := fmt.Fprintf(w, format, args...)
	return err
}
