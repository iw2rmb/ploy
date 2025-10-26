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
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/iw2rmb/ploy/internal/cli/config"
	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

const defaultControlPlanePort = 8443
const defaultSSHPort = 22
const defaultSSHUser = "root"
const remotePKIDir = "/etc/ploy/pki"
const remoteControlPlaneCAPath = remotePKIDir + "/control-plane-ca.pem"
const remoteNodeCertPath = remotePKIDir + "/node.pem"
const remoteNodeKeyPath = remotePKIDir + "/node-key.pem"
const remoteConfigPath = "/etc/ploy/ployd.yaml"

var (
	clusterBootstrapRunner                               = deploy.RunBootstrap
	clusterProvisionHost                                 = deploy.ProvisionHost
	clusterWorkerRegister                                = registerWorker
	clusterHTTPClientFactory descriptorHTTPClientFactory = newDescriptorHTTPClient
	remoteCommandExecutor                                = runRemoteCommand
	remoteFileWriter                                     = writeRemoteFile
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
		return runClusterBootstrap(address.value, userFlag, identity, control, ploydBin, sshPort, stderr)
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

func runClusterBootstrap(address string, userFlag, identity, control, ploydBin stringValue, sshPort intValue, stderr io.Writer) error {
	addr := strings.TrimSpace(address)
	if addr == "" {
		return errors.New("address is required")
	}
	userName := strings.TrimSpace(userFlag.value)
	if userName == "" {
		userName = defaultSSHUser
	}
	identityPath, err := resolveIdentityPath(identity)
	if err != nil {
		return err
	}
	cfg := deploycli.BootstrapConfig{
		Address:       addr,
		Stdout:        stderr,
		Stderr:        stderr,
		Stdin:         os.Stdin,
		WorkstationOS: runtime.GOOS,
		User:          userName,
		IdentityFile:  identityPath,
	}
	if control.set {
		cfg.ControlPlaneURL = strings.TrimSpace(control.value)
	}
	if ploydBin.set {
		cfg.PloydBinaryPath = strings.TrimSpace(ploydBin.value)
	}
	primary := true
	if primary {
		clusterSlug := config.SanitizeID(addr)
		if clusterSlug == "" {
			clusterSlug = config.SanitizeID(fmt.Sprintf("cluster-%s", strings.ReplaceAll(addr, ".", "-")))
		}
		cfg.ClusterID = clusterSlug
		cfg.NodeAddress = addr
		cfg.NodeID = deriveControlPlaneNodeID(addr)
		cfg.Primary = true
	}
	cmd := deploycli.BootstrapCommand{RunBootstrap: clusterBootstrapRunner}
	if err := cmd.Run(context.Background(), cfg); err != nil {
		return err
	}
	if primary {
		if err := captureControlPlaneSecurity(cfg.ClusterID, addr, userName, identityPath, sshPort.value, stderr); err != nil {
			return err
		}
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
	ClusterID       string
	WorkerAddress   string
	User            string
	IdentityFile    string
	PloydBinary     string
	SSHPort         int
	Labels          map[string]string
	Probes          []deploy.WorkerHealthProbe
	DryRun          bool
	ControlPlaneURL string
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
	if cfg.ControlPlaneURL == "" {
		cfg.ControlPlaneURL = baseURL
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
			ScriptArgs:      []string{"--cluster-id", desc.ClusterID},
			ServiceChecks:   []string{"ployd"},
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
	if err := installWorkerArtifacts(cfg, result, stderr); err != nil {
		return err
	}
	if err := updateDescriptorSecurity(desc.ClusterID, "", result.CABundle); err != nil {
		return err
	}
	return renderWorkerJoinResult(stderr, desc.ClusterID, result)
}

func descriptorControlPlaneURL(desc config.Descriptor) (string, error) {
	addr := strings.TrimSpace(desc.Address)
	if addr == "" {
		return "", errors.New("cluster descriptor missing address; re-run 'ploy cluster add'")
	}
	if strings.Contains(addr, "://") {
		return addr, nil
	}
	scheme := strings.TrimSpace(desc.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	switch scheme {
	case "http", "https":
	default:
		return "", fmt.Errorf("invalid control plane scheme %q", scheme)
	}
	host := addr
	if h, p, err := net.SplitHostPort(addr); err == nil {
		port, err := strconv.Atoi(p)
		if err != nil || port <= 0 {
			return "", fmt.Errorf("invalid control plane port %q", p)
		}
		return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(h, strconv.Itoa(port))), nil
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(defaultControlPlanePort))), nil
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

func normalizeSSHUser(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return defaultSSHUser
}

func buildCLISSArgs(identity string, port int) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if trimmed := strings.TrimSpace(identity); trimmed != "" {
		args = append(args, "-i", trimmed)
	}
	if port == 0 {
		port = defaultSSHPort
	}
	if port != defaultSSHPort {
		args = append(args, "-p", strconv.Itoa(port))
	}
	return args
}

func sshTarget(user, host string) string {
	if trimmed := strings.TrimSpace(user); trimmed != "" {
		return fmt.Sprintf("%s@%s", trimmed, strings.TrimSpace(host))
	}
	return strings.TrimSpace(host)
}

func runRemoteCommand(ctx context.Context, target string, sshArgs []string, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	args := append(append([]string(nil), sshArgs...), target, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}
	return cmd.Run()
}

func writeRemoteFile(ctx context.Context, target string, sshArgs []string, remotePath string, mode os.FileMode, data []byte, stderr io.Writer) error {
	cmd := fmt.Sprintf("install -m%04o /dev/stdin %s", mode, remotePath)
	return runRemoteCommand(ctx, target, sshArgs, cmd, bytes.NewReader(data), nil, stderr)
}

func readRemoteFile(ctx context.Context, target string, sshArgs []string, remotePath string, stderr io.Writer) (string, error) {
	var buf bytes.Buffer
	cmd := fmt.Sprintf("cat %s", shellQuote(remotePath))
	if err := runRemoteCommand(ctx, target, sshArgs, cmd, nil, &buf, stderr); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func captureControlPlaneSecurity(clusterID, address, user, identity string, sshPort int, stderr io.Writer) error {
	if strings.EqualFold(os.Getenv("PLOY_SKIP_REMOTE_CA_FETCH"), "true") {
		return nil
	}
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return errors.New("descriptor cluster id required for TLS update")
	}
	ctx := context.Background()
	sshArgs := buildCLISSArgs(identity, sshPort)
	target := sshTarget(user, address)
	ca, err := readRemoteFile(ctx, target, sshArgs, remoteControlPlaneCAPath, stderr)
	if err != nil {
		return fmt.Errorf("fetch control-plane CA: %w", err)
	}
	return updateDescriptorSecurity(trimmed, "https", ca)
}

func deriveControlPlaneNodeID(address string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return unicode.ToLower(r)
		case r >= '0' && r <= '9':
			return r
		case r == '-':
			return r
		default:
			return -1
		}
	}, strings.ReplaceAll(address, ".", "-"))
	if cleaned == "" {
		cleaned = "control"
	}
	return fmt.Sprintf("control-%s", cleaned)
}

func ensureHTTPS(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	switch {
	case strings.HasPrefix(trimmed, "https://"):
		return trimmed
	case strings.HasPrefix(trimmed, "http://"):
		return "https://" + strings.TrimPrefix(trimmed, "http://")
	case trimmed != "":
		return "https://" + trimmed
	default:
		return ""
	}
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
	if caBundle := strings.TrimSpace(desc.CABundle); caBundle != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caBundle)) {
			return nil, nil, fmt.Errorf("cluster descriptor CA bundle invalid")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
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
	CABundle    string                       `json:"ca_bundle"`
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

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
func installWorkerArtifacts(cfg workerProvisionConfig, result nodeJoinResponse, stderr io.Writer) error {
	if cfg.DryRun {
		return nil
	}
	cert := strings.TrimSpace(result.Certificate.CertificatePEM)
	key := strings.TrimSpace(result.Certificate.KeyPEM)
	ca := strings.TrimSpace(result.CABundle)
	if cert == "" || key == "" || ca == "" {
		return nil
	}
	ctx := context.Background()
	user := normalizeSSHUser(cfg.User)
	target := sshTarget(user, cfg.WorkerAddress)
	sshArgs := buildCLISSArgs(cfg.IdentityFile, cfg.SSHPort)
	if err := remoteCommandExecutor(ctx, target, sshArgs, "mkdir -p "+remotePKIDir+" && chmod 700 "+remotePKIDir, nil, nil, stderr); err != nil {
		return fmt.Errorf("prepare remote pki dir: %w", err)
	}
	if err := remoteFileWriter(ctx, target, sshArgs, remoteControlPlaneCAPath, 0o644, []byte(ca), stderr); err != nil {
		return fmt.Errorf("install control-plane ca: %w", err)
	}
	if err := remoteFileWriter(ctx, target, sshArgs, remoteNodeCertPath, 0o644, []byte(cert), stderr); err != nil {
		return fmt.Errorf("install worker certificate: %w", err)
	}
	if err := remoteFileWriter(ctx, target, sshArgs, remoteNodeKeyPath, 0o600, []byte(key), stderr); err != nil {
		return fmt.Errorf("install worker key: %w", err)
	}
	endpoint := ensureHTTPS(cfg.ControlPlaneURL)
	if endpoint != "" {
		escaped := strings.ReplaceAll(endpoint, `"`, `\"`)
		rewrite := fmt.Sprintf(`perl -0pi -e 's|(control_plane:\s*\n\s+endpoint:\s*)\"[^\"]+\"|\\1\"%s\"|' %s`, escaped, remoteConfigPath)
		if err := remoteCommandExecutor(ctx, target, sshArgs, rewrite, nil, nil, stderr); err != nil {
			return fmt.Errorf("update worker config endpoint: %w", err)
		}
	}
	if err := remoteCommandExecutor(ctx, target, sshArgs, "systemctl restart ployd", nil, nil, stderr); err != nil {
		return fmt.Errorf("restart worker ployd: %w", err)
	}
	return nil
}

func updateDescriptorSecurity(clusterID, scheme, caBundle string) error {
	trimmedID := strings.TrimSpace(clusterID)
	if trimmedID == "" {
		return nil
	}
	desc, err := config.LoadDescriptor(trimmedID)
	if err != nil {
		return err
	}
	changed := false
	if trimmed := strings.TrimSpace(scheme); trimmed != "" && desc.Scheme != trimmed {
		desc.Scheme = trimmed
		changed = true
	}
	if trimmed := strings.TrimSpace(caBundle); trimmed != "" && desc.CABundle != trimmed {
		desc.CABundle = trimmed
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = config.SaveDescriptor(desc)
	return err
}
