package deploycli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/iw2rmb/ploy/internal/deploy"
)

var (
	errMissingRunner = errors.New("deploy: bootstrap runner required")
)

// BootstrapConfig encapsulates the adjustable inputs for bootstrap provisioning.
type BootstrapConfig struct {
	User            string
	IdentityFile    string
	Address         string
	ControlPlaneURL string
	PloydBinaryPath string
	Stdout          io.Writer
	Stderr          io.Writer
	Stdin           io.Reader
	WorkstationOS   string
}

// BootstrapCommand prepares deploy.Options and invokes the deployment runner.
type BootstrapCommand struct {
	RunBootstrap      func(context.Context, deploy.Options) error
	LocatePloydBinary func(string) (string, error)
	DefaultIdentity   func() string
}

// Run executes the bootstrap flow using the provided configuration.
func (c BootstrapCommand) Run(ctx context.Context, cfg BootstrapConfig) error {
	runner := c.RunBootstrap
	if runner == nil {
		runner = deploy.RunBootstrap
	}
	if runner == nil {
		return errMissingRunner
	}

	workstationOS := strings.TrimSpace(cfg.WorkstationOS)
	if workstationOS == "" {
		workstationOS = runtime.GOOS
	}

	opts := deploy.Options{}
	opts.User = strings.TrimSpace(cfg.User)
	opts.Address = strings.TrimSpace(cfg.Address)
	opts.ControlPlaneURL = strings.TrimSpace(cfg.ControlPlaneURL)
	opts.WorkstationOS = workstationOS

	identity := strings.TrimSpace(cfg.IdentityFile)
	if identity == "" {
		provider := c.DefaultIdentity
		if provider == nil {
			provider = defaultIdentityPath
		}
		opts.IdentityFile = provider()
	} else {
		opts.IdentityFile = ExpandPath(identity)
	}

	ploydPath := strings.TrimSpace(cfg.PloydBinaryPath)
	if ploydPath != "" {
		opts.PloydBinaryPath = ExpandPath(ploydPath)
	} else {
		locator := c.LocatePloydBinary
		if locator == nil {
			locator = defaultPloydBinaryPath
		}
		path, err := locator(workstationOS)
		if err != nil {
			return err
		}
		opts.PloydBinaryPath = path
	}

	if trimmed := strings.TrimSpace(cfg.ControlPlaneURL); trimmed != "" {
		opts.ControlPlaneURL = trimmed
	} else if opts.ControlPlaneURL == "" {
		opts.ControlPlaneURL = "http://127.0.0.1:9094"
	}
	if opts.Address == "" {
		return errors.New("deploy: address required")
	}
	opts.DescriptorID = opts.Address
	opts.DescriptorAddress = opts.Address
	opts.DescriptorIdentityPath = opts.IdentityFile

	opts.Stdout = cfg.Stdout
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	opts.Stderr = cfg.Stderr
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	opts.Stdin = cfg.Stdin
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}

	if ctx == nil {
		ctx = context.Background()
	}
	return runner(ctx, opts)
}

// ExpandPath resolves a leading tilde to the user home directory.
func ExpandPath(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

// DefaultIdentityPath returns the conventional SSH identity path.
func DefaultIdentityPath() string {
	return defaultIdentityPath()
}

// DefaultPloydBinaryPath locates the ployd binary adjacent to the CLI executable.
func DefaultPloydBinaryPath(workstationOS string) (string, error) {
	return defaultPloydBinaryPath(workstationOS)
}

func defaultIdentityPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "id_rsa")
}

func defaultPloydBinaryPath(workstationOS string) (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate ploy executable: %w", err)
	}
	dir := filepath.Dir(execPath)
	candidates := []string{
		filepath.Join(dir, "ployd"),
	}
	osName := workstationOS
	if osName == "" {
		osName = runtime.GOOS
	}
	if osName == "windows" {
		candidates = append([]string{filepath.Join(dir, "ployd.exe")}, candidates...)
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		return candidate, nil
	}
	return "", errors.New("ploy cluster add: ployd binary not found alongside CLI; provide --ployd-binary")
}
