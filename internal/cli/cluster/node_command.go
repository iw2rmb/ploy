package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
	"github.com/iw2rmb/ploy/internal/deploy"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleNode routes node subcommands.
func handleNode(args []string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	// Handle --help and -h flags to print usage and exit cleanly.
	if common.WantsHelp(args) {
		printNodeUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printNodeUsage(stderr)
		return errors.New("node subcommand required")
	}
	switch args[0] {
	case "add":
		return handleNodeAdd(args[1:], stderr)
	case "ls":
		return handleNodeList(args[1:], stdout, stderr)
	case "inspect":
		return handleNodeInspect(args[1:], stdout, stderr)
	default:
		printNodeUsage(stderr)
		return fmt.Errorf("unknown node subcommand %q", args[0])
	}
}

// printNodeUsage prints the node command usage information.
// This provides a single, consistent usage output for --help, error paths,
// and unknown subcommand handling.
//
// NOTE: Node commands are now accessed via `ploy cluster node` instead of the
// former `ploy node`. This keeps node operations under the existing
// `ploy cluster` command group.
func printNodeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster node <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  add       Add a worker node")
	_, _ = fmt.Fprintln(w, "  ls        List worker nodes")
	_, _ = fmt.Fprintln(w, "  inspect   Inspect a worker node")
}

func handleNodeList(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		_, _ = fmt.Fprintln(stderr, "Usage: ploy cluster node ls")
		return nil
	}
	if len(args) != 0 {
		return fmt.Errorf("node ls takes no arguments")
	}
	ctx := context.Background()
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}
	nodes, err := (sharedclient.ListNodesCommand{Client: client, BaseURL: baseURL}).Run(ctx)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		_, _ = fmt.Fprintln(stdout, "No nodes found.")
		return nil
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "HOST\tNAME\tADDRESS")
	for _, node := range nodes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", node.ID, node.Name, node.IPAddress)
	}
	return tw.Flush()
}

func handleNodeInspect(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		_, _ = fmt.Fprintln(stderr, "Usage: ploy cluster node inspect <name>")
		return nil
	}
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		_, _ = fmt.Fprintln(stderr, "Usage: ploy cluster node inspect <name>")
		return errors.New("node name is required")
	}

	ctx := context.Background()
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}
	nodes, err := (sharedclient.ListNodesCommand{Client: client, BaseURL: baseURL}).Run(ctx)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(args[0])
	var selected *domainapi.Node
	for i := range nodes {
		if nodes[i].Name == name {
			selected = &nodes[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("node %q not found", name)
	}

	diagnostics, err := (sharedclient.ListNodeDiagnosticsCommand{
		Client: client, BaseURL: baseURL, NodeID: selected.ID,
	}).Run(ctx)
	if err != nil {
		return err
	}
	jobs, err := listRunningNodeJobs(ctx, client, baseURL, selected.ID)
	if err != nil {
		return err
	}
	renderNodeInspect(stdout, *selected, diagnostics, jobs)
	return nil
}

func listRunningNodeJobs(ctx context.Context, client *http.Client, baseURL *url.URL, nodeID domaintypes.NodeID) ([]domainapi.JobListItem, error) {
	const pageSize int32 = 100
	status := domaintypes.JobStatusRunning
	jobs := make([]domainapi.JobListItem, 0)
	for offset := int32(0); ; offset += pageSize {
		page, err := (sharedclient.ListJobsCommand{
			Client: client, BaseURL: baseURL, Limit: pageSize, Offset: offset,
			NodeID: &nodeID, Status: &status,
		}).Run(ctx)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, page.Jobs...)
		if int64(len(jobs)) >= page.Total || len(page.Jobs) < int(pageSize) {
			return jobs, nil
		}
	}
}

type nodeDiagnosticDetails struct {
	Concurrency int32 `json:"concurrency"`
}

func renderNodeInspect(out io.Writer, node domainapi.Node, diagnostics []domainapi.NodeDiagnostic, jobs []domainapi.JobListItem) {
	imageRef := "unknown"
	capacity := node.Concurrency
	for _, diagnostic := range diagnostics {
		if diagnostic.Component != "node" {
			continue
		}
		if diagnostic.ImageRef != nil && strings.TrimSpace(*diagnostic.ImageRef) != "" {
			imageRef = strings.TrimSpace(*diagnostic.ImageRef)
		}
		var details nodeDiagnosticDetails
		if json.Unmarshal(diagnostic.Details, &details) == nil && details.Concurrency > 0 {
			capacity = details.Concurrency
		}
		break
	}

	heartbeat := "unknown"
	if node.LastHeartbeat != nil && strings.TrimSpace(*node.LastHeartbeat) != "" {
		heartbeat = strings.TrimSpace(*node.LastHeartbeat)
	}
	available := int(capacity) - len(jobs)
	if available < 0 {
		available = 0
	}

	_, _ = fmt.Fprintf(out, "Name:             %s\n", node.Name)
	_, _ = fmt.Fprintf(out, "Host:             %s\n", node.ID)
	_, _ = fmt.Fprintf(out, "Address:          %s\n", node.IPAddress)
	_, _ = fmt.Fprintf(out, "Image:            %s\n", imageRef)
	_, _ = fmt.Fprintf(out, "Last heartbeat:   %s\n", heartbeat)
	_, _ = fmt.Fprintf(out, "Queue:            %d/%d active (%d available)\n", len(jobs), capacity, available)
	_, _ = fmt.Fprintf(out, "CPU available:    %s\n", formatCPU(node.CPUFreeMillis, node.CPUTotalMillis))
	_, _ = fmt.Fprintf(out, "Memory available: %s\n", formatBytes(node.MemFreeBytes, node.MemTotalBytes))
	_, _ = fmt.Fprintf(out, "Disk available:   %s\n", formatBytes(node.DiskFreeBytes, node.DiskTotalBytes))
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Active jobs:")
	if len(jobs) == 0 {
		_, _ = fmt.Fprintln(out, "  none")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "JOB ID\tNAME\tMIG\tRUN ID\tREPO ID\tIMAGE")
	for _, job := range jobs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			job.JobID, job.Name, job.MigName, job.RunID, job.RepoID, job.JobImage)
	}
	_ = tw.Flush()
}

func formatCPU(free, total int32) string {
	if total <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.2f / %.2f cores", float64(free)/1000, float64(total)/1000)
}

func formatBytes(free, total int64) string {
	if total <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("%s / %s", formatByteCount(free), formatByteCount(total))
}

func formatByteCount(value int64) string {
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor := unit
	unitIndex := 0
	for next := divisor * unit; value >= next && unitIndex < 3; next = divisor * unit {
		divisor = next
		unitIndex++
	}
	return fmt.Sprintf("%.2f %ciB", float64(value)/float64(divisor), "KMGT"[unitIndex])
}

// printNodeAddUsage prints usage information for the node add command.
// NOTE: Node add is now accessed via `ploy cluster node add`.
func printNodeAddUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster node add --address <ip> --server-url <url>")
}

// handleNodeAdd validates required flags for adding a worker node.
func handleNodeAdd(args []string, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printNodeAddUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("node add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		address      common.StringValue
		serverURL    common.StringValue
		identity     common.StringValue
		userFlag     common.StringValue
		ploydNodeBin common.StringValue
		sshPort      common.IntValue
		dryRun       bool
	)
	fs.Var(&address, "address", "Node IP or hostname")
	fs.Var(&serverURL, "server-url", "Ploy server URL (required; e.g. https://<server-host>:8443)")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&ploydNodeBin, "ployd-node-binary", "Path to the ployd-node binary uploaded during provisioning (default: alongside the CLI)")
	fs.Var(&sshPort, "ssh-port", "SSH port for node provisioning (default: 22)")
	fs.BoolVar(&dryRun, "dry-run", false, "Validate inputs without performing provisioning")

	if err := common.ParseFlagSet(fs, args, func() { printNodeAddUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printNodeAddUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !address.IsSet || strings.TrimSpace(address.Value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("address is required")
	}

	nodeCfg := nodeAddConfig{
		Address:         address.Value,
		ServerURL:       serverURL.Value,
		User:            userFlag.Value,
		IdentityFile:    identity.Value,
		PloydNodeBinary: ploydNodeBin.Value,
		SSHPort:         sshPort.Value,
		DryRun:          dryRun,
	}

	return runNodeAdd(nodeCfg, stderr)
}

type nodeAddConfig struct {
	Address         string
	ServerURL       string
	User            string
	IdentityFile    string
	PloydNodeBinary string
	SSHPort         int
	DryRun          bool
}

func runNodeAdd(cfg nodeAddConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}
	ctx := context.Background()

	// Resolve default paths
	identityPath, err := common.ResolveIdentityPath(common.StringValue{IsSet: cfg.IdentityFile != "", Value: cfg.IdentityFile})
	if err != nil {
		return fmt.Errorf("node add: %w", err)
	}

	ploydNodeBinaryPath, err := resolvePloydNodeBinaryPath(common.StringValue{IsSet: cfg.PloydNodeBinary != "", Value: cfg.PloydNodeBinary})
	if err != nil {
		return fmt.Errorf("node add: %w", err)
	}

	user := cfg.User
	if strings.TrimSpace(user) == "" {
		user = deploy.DefaultRemoteUser
	}

	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}
	if err := common.ValidateSSHPort(sshPort); err != nil {
		return fmt.Errorf("node add: %w", err)
	}

	if cfg.DryRun {
		_, _ = fmt.Fprintln(stderr, "[DRY RUN] Validating node add configuration...")
	}

	_, _ = fmt.Fprintln(stderr, "Adding node")
	_, _ = fmt.Fprintf(stderr, "  Node Address: %s\n", cfg.Address)
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydNodeBinaryPath)

	// Generate node ID
	nodeID := deploy.GenerateNodeID()
	_, _ = fmt.Fprintf(stderr, "Generated node ID: %s\n", nodeID)

	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		return errors.New("node add: server-url is required")
	}

	// Early exit for dry-run mode after validation.
	if cfg.DryRun {
		_, _ = fmt.Fprintln(stderr, "\n[DRY RUN] Validation complete. Provisioning would proceed with:")
		_, _ = fmt.Fprintf(stderr, "  Server URL: %s\n", serverURL)
		_, _ = fmt.Fprintf(stderr, "  Generated Node ID: %s\n", nodeID)
		_, _ = fmt.Fprintln(stderr, "\nAll validations passed. No actual provisioning performed.")
		return nil
	}

	// Request bootstrap token from server
	_, _ = fmt.Fprintln(stderr, "Requesting bootstrap token from server...")
	bootstrapToken, expiresAt, err := requestBootstrapToken(ctx, serverURL, nodeID)
	if err != nil {
		return fmt.Errorf("node add: request bootstrap token: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "Bootstrap token received (expires: %s)\n", expiresAt.Format(time.RFC3339))

	// Get CA certificate for TLS verification (node will need this to verify server)
	_, _ = fmt.Fprintln(stderr, "Fetching CA certificate for TLS verification...")
	caCert, err := fetchCACertificate(ctx, serverURL, user, sshPort, identityPath, nil)
	if err != nil {
		// For now, we'll allow continuing without CA cert if it fails
		// The node will use system trust store
		_, _ = fmt.Fprintf(stderr, "Warning: could not fetch CA certificate: %v\n", err)
		_, _ = fmt.Fprintln(stderr, "Node will use system trust store for TLS verification")
		caCert = ""
	}

	// Prepare environment variables for bootstrap script
	scriptEnv := map[string]string{
		"NODE_ID":              nodeID,
		"NODE_ADDRESS":         cfg.Address,
		"BOOTSTRAP_PRIMARY":    "false",
		"PLOY_BOOTSTRAP_TOKEN": bootstrapToken,
		"PLOY_SERVER_URL":      serverURL,
	}
	if caCert != "" {
		scriptEnv["PLOY_CA_CERT_PEM"] = caCert
	}

	// Provision the node host
	_, _ = fmt.Fprintln(stderr, "Installing ployd-node binary and bootstrapping node...")
	provisionOpts := deploy.ProvisionOptions{
		Host:            cfg.Address,
		Address:         cfg.Address,
		User:            user,
		Port:            sshPort,
		IdentityFile:    identityPath,
		PloydBinaryPath: ploydNodeBinaryPath,
		Stdout:          os.Stdout,
		Stderr:          stderr,
		ScriptEnv:       scriptEnv,
		ScriptArgs:      []string{"--node-id", nodeID, "--node-address", cfg.Address},
		ServiceChecks:   []string{"ployd-node"},
	}

	if err := deploy.ProvisionHost(ctx, provisionOpts); err != nil {
		return fmt.Errorf("node add: provision host: %w", err)
	}

	_, _ = fmt.Fprintln(stderr, "\nNode provisioning complete!")
	_, _ = fmt.Fprintf(stderr, "Node ID: %s\n", nodeID)
	_, _ = fmt.Fprintf(stderr, "Node address: %s\n", cfg.Address)
	_, _ = fmt.Fprintln(stderr, "\nThe node is now connected and ready to accept runs.")

	return nil
}

// resolvePloydNodeBinaryPath locates the ployd-node binary adjacent to the CLI.
func resolvePloydNodeBinaryPath(v common.StringValue) (string, error) {
	if v.IsSet {
		path := common.ExpandPath(v.Value)
		if err := common.ValidateFileReadable(path); err != nil {
			return "", fmt.Errorf("ployd-node binary: %w", err)
		}
		return path, nil
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate ploy executable: %w", err)
	}
	dir := filepath.Dir(execPath)
	osName := runtime.GOOS
	candidates := make([]string, 0, 3)
	if osName != "linux" {
		candidates = append(candidates, filepath.Join(dir, "ployd-node-linux"))
	}
	if osName == "windows" {
		candidates = append(candidates, filepath.Join(dir, "ployd-node.exe"))
	}
	candidates = append(candidates, filepath.Join(dir, "ployd-node"))
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	// Used by `ploy cluster node add` (flag: --ployd-node-binary).
	return "", errors.New("ployd-node binary not found alongside CLI; provide --ployd-node-binary (cluster node add)")
}

// requestBootstrapToken requests a short-lived bootstrap token from the server for node provisioning.
func requestBootstrapToken(ctx context.Context, serverURL, nodeID string) (token string, expiresAt time.Time, err error) {
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("resolve control plane: %w", err)
	}

	reqBody := domainapi.CreateBootstrapTokenRequest{
		NodeID:           domaintypes.NodeID(nodeID),
		ExpiresInMinutes: 15,
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/bootstrap/tokens"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("POST %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", time.Time{}, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result domainapi.CreateBootstrapTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decode response: %w", err)
	}

	return result.Token, result.ExpiresAt, nil
}

// fetchCACertificate reads the CA certificate from the server host over SSH so
// the node can verify the server during bootstrap (before it has mTLS credentials).
//
// The CA cert is expected at /etc/ploy/pki/ca.crt (written during server deploy).
// If serverURL is not https, an empty string is returned.
func fetchCACertificate(ctx context.Context, serverURL, sshUser string, sshPort int, identityPath string, runner deploy.Runner) (string, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(serverURL)), "http://") {
		return "", nil
	}

	u, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || u.Hostname() == "" {
		// url.Parse treats URLs without a scheme as paths. Retry with an https scheme.
		u, err = url.Parse("https://" + strings.TrimSpace(serverURL))
	}
	if err != nil {
		return "", fmt.Errorf("parse server URL: %w", err)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("parse server URL: missing host")
	}
	if strings.TrimSpace(sshUser) == "" {
		sshUser = deploy.DefaultRemoteUser
	}
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}
	if runner == nil {
		runner = deploy.NewSystemRunner()
	}

	sshArgs := deploy.BuildSSHArgs(identityPath, sshPort)
	target := fmt.Sprintf("%s@%s", sshUser, host)
	out := &strings.Builder{}
	args := append(append([]string(nil), sshArgs...), target, "cat /etc/ploy/pki/ca.crt")
	if err := runner.Run(ctx, "ssh", args, nil, deploy.IOStreams{Stdout: out, Stderr: io.Discard}); err != nil {
		return "", fmt.Errorf("read CA cert from %s: %w", host, err)
	}
	return out.String(), nil
}
