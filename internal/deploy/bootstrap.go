package deploy

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/bootstrap"
	"github.com/iw2rmb/ploy/internal/cli/config"
)

const (
	// DefaultRemoteUser is applied when no remote user is provided.
	DefaultRemoteUser = "root"
	// DefaultSSHPort is used when no SSH port is specified.
	DefaultSSHPort = 22
	// remotePloydBinaryPath is where the ployd binary is installed on the target host.
	remotePloydBinaryPath = "/usr/local/bin/ployd"
	// defaultControlPlaneEndpointValue is used when no control plane URL is provided.
	defaultControlPlaneEndpointValue = "http://127.0.0.1:9094"
)

// Options configure bootstrap execution.
type Options struct {
	Host                   string
	Address                string
	User                   string
	Port                   int
	IdentityFile           string
	Stdout                 io.Writer
	Stderr                 io.Writer
	Runner                 Runner
	PloydBinaryPath        string
	ControlPlaneURL        string
	Clock                  func() time.Time
	Stdin                  io.Reader
	WorkstationOS          string
	DescriptorID           string
	DescriptorAddress      string
	DescriptorIdentityPath string
	ClusterID              string
	InitialWorkers         []string
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

	address := strings.TrimSpace(opts.Address)
	if address == "" {
		return errors.New("bootstrap: address required")
	}
	opts.Address = address

	user := strings.TrimSpace(opts.User)
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

	displayTarget := address
	ploydBinary := strings.TrimSpace(opts.PloydBinaryPath)
	if ploydBinary == "" {
		return errors.New("bootstrap: ployd binary path required")
	}

	envVars := map[string]string{
		"PLOY_CONTROL_PLANE_ENDPOINT": defaultControlPlaneEndpoint(opts.ControlPlaneURL),
		"PLOY_BOOTSTRAP_VERSION":      bootstrap.Version,
	}

	provisionOpts := ProvisionOptions{
		Host:            address,
		Address:         address,
		User:            user,
		Port:            port,
		IdentityFile:    opts.IdentityFile,
		PloydBinaryPath: ploydBinary,
		Runner:          runner,
		Stdout:          stdout,
		Stderr:          stderr,
		ScriptEnv:       envVars,
		ServiceChecks:   []string{"ployd"},
	}

	if err := ProvisionHost(ctx, provisionOpts); err != nil {
		return err
	}

	descriptorID := strings.TrimSpace(opts.DescriptorID)
	if descriptorID == "" {
		descriptorID = address
	}
	desc := config.Descriptor{
		ClusterID:       descriptorID,
		Address:         strings.TrimSpace(opts.DescriptorAddress),
		SSHIdentityPath: strings.TrimSpace(opts.DescriptorIdentityPath),
	}
	if desc.Address == "" {
		desc.Address = address
	}
	if desc.SSHIdentityPath == "" {
		desc.SSHIdentityPath = opts.IdentityFile
	}
	saved, err := config.SaveDescriptor(desc)
	if err != nil {
		return fmt.Errorf("bootstrap: save descriptor: %w", err)
	}
	if err := config.SetDefault(saved.ClusterID); err != nil {
		return fmt.Errorf("bootstrap: set default descriptor: %w", err)
	}

	if _, err := fmt.Fprintf(stderr, "Bootstrap completed for %s.\n", displayTarget); err != nil {
		return fmt.Errorf("bootstrap: write completion message: %w", err)
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

type configureWorkstationOptions struct {
	ClusterID   string
	CAPath      string
	BeaconIP    string
	ResolverDir string
	GOOS        string
	Runner      Runner
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
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
