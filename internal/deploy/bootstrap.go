package deploy

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

type clusterPKIState struct {
	CurrentCA  CABundle
	Descriptor config.Descriptor
	CABundle   string
}

const (
	// DefaultRemoteUser is applied when no remote user is provided.
	DefaultRemoteUser = "root"
	// DefaultSSHPort is used when no SSH port is specified.
	DefaultSSHPort = 22
	// remotePloydBinaryPath is where the ployd binary is installed on the target host.
	remotePloydBinaryPath = "/usr/local/bin/ployd"
	// defaultControlPlaneEndpointValue is used when no control plane URL is provided.
	defaultControlPlaneEndpointValue = "https://control.example.com"
)

// Options configure bootstrap execution.
type Options struct {
	Host                string
	Address             string
	User                string
	Port                int
	IdentityFile        string
	Stdout              io.Writer
	Stderr              io.Writer
	Runner              Runner
	ClusterID           string
	EtcdClient          *clientv3.Client
	PloydBinaryPath     string
	InitialBeacons      []string
	InitialWorkers      []string
	BeaconURL           string
	ControlPlaneURL     string
	APIKey              string
	Clock               func() time.Time
	Stdin               io.Reader
	WorkstationOS       string
	ResolverDir         string
	EtcdEndpoints       []string
	AdminAuthorizedKeys []string
	UserAuthorizedKeys  []string
}

// IOStreams represents command IO endpoints.
type IOStreams struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Runner executes commands with the rendered script.
type Runner interface {
	Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error
}

// RunnerFunc adapts a function to the Runner interface.
type RunnerFunc func(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error

// Run executes the underlying function.
func (fn RunnerFunc) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
	return fn(ctx, command, args, stdin, streams)
}

type systemRunner struct{}

func (systemRunner) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
	cmd := exec.CommandContext(ctx, command, args...)
	if streams.Stdout != nil {
		cmd.Stdout = streams.Stdout
	}
	if streams.Stderr != nil {
		cmd.Stderr = streams.Stderr
	}
	if stdin != nil {
		cmd.Stdin = stdin
	} else {
		cmd.Stdin = os.Stdin
	}
	return cmd.Run()
}

// RunBootstrap orchestrates remote installation via SSH and finalises PKI metadata locally.
func RunBootstrap(ctx context.Context, opts Options) error {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	goos := strings.TrimSpace(opts.WorkstationOS)
	if goos == "" {
		goos = runtime.GOOS
	}
	resolverDir := strings.TrimSpace(opts.ResolverDir)
	if resolverDir == "" {
		resolverDir = "/etc/resolver"
	}

	clusterID := strings.TrimSpace(opts.ClusterID)
	if clusterID == "" {
		return errors.New("bootstrap: cluster id required")
	}
	opts.ClusterID = clusterID

	opts.BeaconURL = strings.TrimSpace(opts.BeaconURL)
	opts.ControlPlaneURL = strings.TrimSpace(opts.ControlPlaneURL)
	opts.APIKey = strings.TrimSpace(opts.APIKey)
	if opts.APIKey == "" {
		key, err := generateAPIKey()
		if err != nil {
			return fmt.Errorf("bootstrap: generate api key: %w", err)
		}
		opts.APIKey = key
	}

	adminKeys := normalizedAuthorizedKeys(opts.AdminAuthorizedKeys)
	if len(adminKeys) == 0 {
		return errors.New("bootstrap: admin authorized keys required")
	}
	userKeys := normalizedAuthorizedKeys(opts.UserAuthorizedKeys)
	if len(userKeys) == 0 {
		return errors.New("bootstrap: user authorized keys required")
	}

	if opts.BeaconURL == "" {
		return errors.New("bootstrap: beacon url required")
	}
	if !hasNonEmpty(opts.InitialBeacons) {
		return errors.New("bootstrap: at least one beacon id required")
	}

	user := opts.User
	if user == "" {
		user = DefaultRemoteUser
	}
	port := opts.Port
	if port == 0 {
		port = DefaultSSHPort
	}

	runner := opts.Runner
	if runner == nil {
		runner = systemRunner{}
	}

	if opts.Host == "" {
		return errors.New("bootstrap: host required")
	}

	connectHost := strings.TrimSpace(opts.Address)
	if connectHost == "" {
		connectHost = strings.TrimSpace(opts.Host)
	}
	if connectHost == "" {
		return errors.New("bootstrap: address required")
	}

	displayTarget := opts.Host
	if displayTarget == "" {
		displayTarget = connectHost
	} else if opts.Address != "" && opts.Address != opts.Host {
		displayTarget = fmt.Sprintf("%s (%s)", opts.Host, opts.Address)
	}

	ploydBinary := strings.TrimSpace(opts.PloydBinaryPath)
	if ploydBinary == "" {
		return errors.New("bootstrap: ployd binary path required")
	}

	adminPayload := encodeAuthorizedKeys(adminKeys)
	if adminPayload == "" {
		return errors.New("bootstrap: admin authorized keys payload empty")
	}
	userPayload := encodeAuthorizedKeys(userKeys)
	if userPayload == "" {
		return errors.New("bootstrap: user authorized keys payload empty")
	}

	envVars := map[string]string{
		"PLOY_CONTROL_PLANE_ENDPOINT": defaultControlPlaneEndpoint(opts.ControlPlaneURL),
		"PLOY_SSH_ADMIN_KEYS_B64":     adminPayload,
		"PLOY_SSH_USER_KEYS_B64":      userPayload,
	}

	provisionOpts := ProvisionOptions{
		Host:            opts.Host,
		Address:         connectHost,
		User:            user,
		Port:            port,
		IdentityFile:    opts.IdentityFile,
		PloydBinaryPath: ploydBinary,
		Runner:          runner,
		Stdout:          stdout,
		Stderr:          stderr,
		Mode:            ProvisionModeBeacon,
		ScriptEnv:       envVars,
		ServiceChecks:   []string{"etcd", "ployd"},
	}

	if err := ProvisionHost(ctx, provisionOpts); err != nil {
		return err
	}

	var stopTunnel func() error
	if opts.EtcdClient == nil {
		endpoint, closer, err := startEtcdTunnel(ctx, opts, port, stderr, stdin)
		if err != nil {
			return err
		}
		stopTunnel = closer
		opts.EtcdEndpoints = []string{endpoint}
		defer func() {
			if stopTunnel != nil {
				_ = stopTunnel()
			}
		}()
	}

	client := opts.EtcdClient
	var closeClient bool
	if client == nil {
		if len(opts.EtcdEndpoints) == 0 {
			return errors.New("bootstrap: etcd endpoints required to finalise PKI")
		}
		dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var err error
		client, err = connectEtcd(dialCtx, opts.EtcdEndpoints)
		if err != nil {
			return fmt.Errorf("bootstrap: connect etcd: %w", err)
		}
		closeClient = true
	}
	if client != nil {
		opts.EtcdClient = client
	}

	state, err := bootstrapClusterPKI(ctx, clusterID, opts)
	if err != nil {
		if closeClient {
			_ = client.Close()
		}
		return err
	}

	if closeClient {
		_ = client.Close()
	}

	if err := configureWorkstation(ctx, configureWorkstationOptions{
		ClusterID:   clusterID,
		CAPath:      state.CABundle,
		BeaconIP:    opts.Address,
		Runner:      runner,
		Stdout:      stdout,
		Stderr:      stderr,
		Stdin:       stdin,
		GOOS:        goos,
		ResolverDir: resolverDir,
	}); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stderr, "Bootstrap completed for %s.\n", displayTarget); err != nil {
		return fmt.Errorf("bootstrap: write completion message: %w", err)
	}
	if _, err := fmt.Fprintf(stderr, "Cluster %s PKI bootstrapped (CA version %s).\n", clusterID, state.CurrentCA.Version); err != nil {
		return fmt.Errorf("bootstrap: write PKI completion message: %w", err)
	}
	return nil
}

func buildSSHArgs(identity string, port int) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if trimmed := strings.TrimSpace(identity); trimmed != "" {
		args = append(args, "-i", trimmed)
	}
	if port != DefaultSSHPort {
		args = append(args, "-p", strconv.Itoa(port))
	}
	return args
}

func buildScpArgs(identity string, port int) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if trimmed := strings.TrimSpace(identity); trimmed != "" {
		args = append(args, "-i", trimmed)
	}
	if port != DefaultSSHPort {
		args = append(args, "-P", strconv.Itoa(port))
	}
	return args
}

func randomHexString(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("bootstrap: random length must be positive")
	}
	buf := make([]byte, (length+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("bootstrap: random token: %w", err)
	}
	hexStr := hex.EncodeToString(buf)
	if len(hexStr) > length {
		hexStr = hexStr[:length]
	}
	return hexStr, nil
}

func defaultControlPlaneEndpoint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultControlPlaneEndpointValue
	}
	return value
}

func bootstrapClusterPKI(ctx context.Context, clusterID string, opts Options) (*clusterPKIState, error) {
	clusterID = strings.TrimSpace(clusterID)
	client := opts.EtcdClient
	if client == nil {
		return nil, errors.New("bootstrap: etcd client required")
	}
	if strings.TrimSpace(opts.BeaconURL) == "" {
		return nil, errors.New("bootstrap: beacon url required")
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("bootstrap: api key required")
	}
	beaconIDs := normalizeNodeIDs(opts.InitialBeacons)
	if len(beaconIDs) == 0 {
		return nil, errors.New("bootstrap: at least one beacon id required")
	}
	workerIDs := normalizeNodeIDs(opts.InitialWorkers)

	manager, err := NewCARotationManager(client, clusterID)
	if err != nil {
		return nil, err
	}

	clock := opts.Clock
	var requestedAt time.Time
	if clock != nil {
		requestedAt = clock().UTC()
	} else {
		requestedAt = time.Now().UTC()
	}

	state, err := manager.Bootstrap(ctx, BootstrapOptions{
		BeaconIDs:   beaconIDs,
		WorkerIDs:   workerIDs,
		RequestedAt: requestedAt,
	})
	if err != nil {
		return nil, err
	}

	caPath, err := config.CABundlePath(clusterID)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: resolve ca bundle path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(caPath), 0o755); err != nil {
		return nil, fmt.Errorf("bootstrap: create config directory: %w", err)
	}
	if err := os.WriteFile(caPath, []byte(state.CurrentCA.CertificatePEM), 0o600); err != nil {
		return nil, fmt.Errorf("bootstrap: write ca bundle: %w", err)
	}

	beaconURL := strings.TrimSpace(opts.BeaconURL)
	controlPlaneURL := strings.TrimSpace(opts.ControlPlaneURL)
	apiKey := strings.TrimSpace(opts.APIKey)

	descriptor := config.Descriptor{
		ID:              clusterID,
		BeaconURL:       beaconURL,
		ControlPlaneURL: controlPlaneURL,
		APIKey:          apiKey,
		CABundlePath:    caPath,
		TrustBundlePath: caPath,
		Version:         state.CurrentCA.Version,
	}
	saved, err := config.SaveDescriptor(descriptor)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: save descriptor: %w", err)
	}
	if err := config.SetDefault(saved.ID); err != nil {
		return nil, fmt.Errorf("bootstrap: set default descriptor: %w", err)
	}

	return &clusterPKIState{
		CurrentCA:  state.CurrentCA,
		Descriptor: saved,
		CABundle:   caPath,
	}, nil
}

func hasNonEmpty(values []string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// normalizedAuthorizedKeys returns a trimmed list of authorized key entries without blanks.
func normalizedAuthorizedKeys(keys []string) []string {
	clean := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		clean = append(clean, trimmed)
	}
	return clean
}

// encodeAuthorizedKeys returns a base64 payload for the supplied authorized keys slice.
func encodeAuthorizedKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	payload := strings.Join(keys, "\n")
	if !strings.HasSuffix(payload, "\n") {
		payload += "\n"
	}
	return base64.StdEncoding.EncodeToString([]byte(payload))
}

type configureWorkstationOptions struct {
	ClusterID   string
	CAPath      string
	BeaconIP    string
	Runner      Runner
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
	GOOS        string
	ResolverDir string
}

func configureWorkstation(ctx context.Context, cfg configureWorkstationOptions) error {
	if cfg.CAPath == "" {
		return errors.New("bootstrap: CA path missing for workstation configuration")
	}
	if cfg.Runner == nil {
		cfg.Runner = systemRunner{}
	}
	if err := installWorkstationCA(ctx, cfg); err != nil {
		return err
	}
	if err := ensureResolverRecord(ctx, cfg); err != nil {
		return err
	}
	return nil
}

func installWorkstationCA(ctx context.Context, cfg configureWorkstationOptions) error {
	switch cfg.GOOS {
	case "darwin":
		return installMacSystemCA(ctx, cfg)
	case "linux":
		return installLinuxSystemCA(ctx, cfg)
	default:
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Skipping system CA install: unsupported OS %s\n", cfg.GOOS)
		}
		return nil
	}
}

func installMacSystemCA(ctx context.Context, cfg configureWorkstationOptions) error {
	const systemKeychain = "/Library/Keychains/System.keychain"
	commonName := fmt.Sprintf("ploy-%s-root", cfg.ClusterID)
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Installing cluster CA into macOS system keychain (sudo).\n")
	}
	deleteArgs := []string{"security", "delete-certificate", "-c", commonName, systemKeychain}
	if err := runCommand(ctx, cfg.Runner, "sudo", deleteArgs, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			if cfg.Stderr != nil {
				_, _ = fmt.Fprintf(cfg.Stderr, "Warning: could not remove existing certificate %s: %v\n", commonName, err)
			}
		}
	}
	addArgs := []string{"security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", systemKeychain, cfg.CAPath}
	if err := runCommand(ctx, cfg.Runner, "sudo", addArgs, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Warning: failed to import cluster CA into System.keychain (continuing): %v\n", err)
		}
		return nil
	}
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "System keychain updated with cluster CA %s.\n", commonName)
	}
	return nil
}

func installLinuxSystemCA(ctx context.Context, cfg configureWorkstationOptions) error {
	dest := filepath.Join("/usr/local/share/ca-certificates", fmt.Sprintf("ploy-%s.crt", cfg.ClusterID))
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Installing cluster CA into system trust store (sudo).\n")
	}
	if err := runCommand(ctx, cfg.Runner, "sudo", []string{"install", "-m0644", cfg.CAPath, dest}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		return fmt.Errorf("bootstrap: install CA bundle into %s: %w", dest, err)
	}
	if _, err := exec.LookPath("update-ca-certificates"); err == nil {
		if err := runCommand(ctx, cfg.Runner, "sudo", []string{"update-ca-certificates"}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
			return fmt.Errorf("bootstrap: update system CAs: %w", err)
		}
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintln(cfg.Stderr, "System trust store refreshed via update-ca-certificates.")
		}
		return nil
	}
	if _, err := exec.LookPath("update-ca-trust"); err == nil {
		if err := runCommand(ctx, cfg.Runner, "sudo", []string{"update-ca-trust", "extract"}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
			return fmt.Errorf("bootstrap: extract system CAs: %w", err)
		}
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintln(cfg.Stderr, "System trust store refreshed via update-ca-trust extract.")
		}
		return nil
	}
	return errors.New("bootstrap: no system CA refresh tool found (expected update-ca-certificates or update-ca-trust)")
}

func ensureResolverRecord(ctx context.Context, cfg configureWorkstationOptions) error {
	if cfg.GOOS != "darwin" {
		return nil
	}
	resolverPath := filepath.Join(cfg.ResolverDir, "ploy")
	if _, err := os.Stat(resolverPath); err == nil {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry already exists at %s; skipping.\n", resolverPath)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("bootstrap: check resolver entry: %w", err)
	}

	nameserver := strings.TrimSpace(cfg.BeaconIP)
	if nameserver == "" {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry %s missing but beacon address not provided; add manually to point to cluster beacon.\n", resolverPath)
		}
		return nil
	}

	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry %s not found. This directs *.ploy lookups to %s.\n", resolverPath, nameserver)
	}
	proceed, err := promptYesNo(cfg.Stdin, cfg.Stderr, "Create resolver entry now (requires sudo)? [y/N]: ")
	if err != nil {
		return fmt.Errorf("bootstrap: resolver prompt: %w", err)
	}
	if !proceed {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Skipping resolver configuration. Add %s manually with `nameserver %s`.\n", resolverPath, nameserver)
		}
		return nil
	}

	tmpFile, err := os.CreateTemp("", "ploy-resolver-*")
	if err != nil {
		return fmt.Errorf("bootstrap: create resolver temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	content := fmt.Sprintf("nameserver %s\n", nameserver)
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("bootstrap: write resolver temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("bootstrap: close resolver temp file: %w", err)
	}

	if err := runCommand(ctx, cfg.Runner, "sudo", []string{"mkdir", "-p", cfg.ResolverDir}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		return fmt.Errorf("bootstrap: prepare resolver directory: %w", err)
	}
	if err := runCommand(ctx, cfg.Runner, "sudo", []string{"install", "-m0644", tmpFile.Name(), resolverPath}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		return fmt.Errorf("bootstrap: install resolver entry: %w", err)
	}
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry %s written with nameserver %s.\n", resolverPath, nameserver)
	}
	return nil
}

func promptYesNo(in io.Reader, out io.Writer, message string) (bool, error) {
	if out != nil {
		if _, err := fmt.Fprint(out, message); err != nil {
			return false, err
		}
	}
	if in == nil {
		return false, nil
	}
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func runCommand(ctx context.Context, runner Runner, command string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	streams := IOStreams{Stdout: stdout, Stderr: stderr}
	return runner.Run(ctx, command, args, stdin, streams)
}

func startEtcdTunnel(ctx context.Context, opts Options, port int, stderr io.Writer, stdin io.Reader) (string, func() error, error) {
	connectHost := strings.TrimSpace(opts.Address)
	if connectHost == "" {
		connectHost = strings.TrimSpace(opts.Host)
	}
	if connectHost == "" {
		return "", nil, errors.New("bootstrap: address required for etcd tunnel")
	}
	user := opts.User
	if user == "" {
		user = DefaultRemoteUser
	}
	target := connectHost
	if user != "" {
		target = fmt.Sprintf("%s@%s", user, connectHost)
	}

	localPort, err := allocateLocalPort()
	if err != nil {
		return "", nil, fmt.Errorf("bootstrap: allocate tunnel port: %w", err)
	}

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if opts.IdentityFile != "" {
		args = append(args, "-i", opts.IdentityFile)
	}
	if port != DefaultSSHPort {
		args = append(args, "-p", strconv.Itoa(port))
	}
	args = append(args, "-L", fmt.Sprintf("%d:127.0.0.1:2379", localPort), target, "-N")

	tunnelCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(tunnelCtx, "ssh", args...)
	if stdin != nil {
		cmd.Stdin = stdin
	} else {
		cmd.Stdin = os.Stdin
	}
	if stderr != nil {
		cmd.Stdout = stderr
		cmd.Stderr = stderr
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return "", nil, fmt.Errorf("bootstrap: start etcd tunnel: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	if err := waitForLocalPort(tunnelCtx, localPort); err != nil {
		cancel()
		select {
		case waitErr := <-waitCh:
			if waitErr != nil {
				err = fmt.Errorf("%v: %w", err, waitErr)
			}
		default:
		}
		return "", nil, err
	}

	stop := func() error {
		cancel()
		return <-waitCh
	}

	return fmt.Sprintf("http://127.0.0.1:%d", localPort), stop, nil
}

func allocateLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	addr := l.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

func waitForLocalPort(ctx context.Context, port int) error {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bootstrap: establish etcd tunnel: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("bootstrap: etcd tunnel cancelled: %w", ctx.Err())
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func connectEtcd(ctx context.Context, endpoints []string) (*clientv3.Client, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("bootstrap: etcd endpoints required")
	}
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
		Context:     ctx,
	}
	client, err := clientv3.New(cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func generateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
