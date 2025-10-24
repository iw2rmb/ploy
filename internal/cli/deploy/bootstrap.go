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

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/iw2rmb/ploy/internal/deploy"
)

const (
	DefaultDomainSuffix      = ".ploy"
	DefaultClusterIDAlphabet = "0123456789abcdef"
	DefaultClusterIDLength   = 16
	DefaultWorkerIDAlphabet  = "0123456789abcdef"
	DefaultWorkerIDLength    = 4
	DefaultAPIKeyAlphabet    = "0123456789abcdef"
	DefaultAPIKeyLength      = 64
)

var (
	ErrBeaconURLRequired           = errors.New("beacon-url is required")
	ErrAPIKeyRequired              = errors.New("api-key is required")
	ErrInitialBeaconIDMissing      = errors.New("at least one beacon-id is required")
	ErrAdminAuthorizedKeysRequired = errors.New("admin-authorized-keys is required")
	ErrUserAuthorizedKeysRequired  = errors.New("user-authorized-keys is required")
	errMissingRunner               = errors.New("deploy: bootstrap runner required")
)

// BootstrapConfig encapsulates the adjustable inputs for bootstrap provisioning.
type BootstrapConfig struct {
	User                    string
	IdentityFile            string
	Address                 string
	ControlPlaneURL         string
	BeaconURL               string
	PloydBinaryPath         string
	Stdout                  io.Writer
	Stderr                  io.Writer
	Stdin                   io.Reader
	WorkstationOS           string
	AdminAuthorizedKeysPath string
	UserAuthorizedKeysPath  string
}

// BootstrapCommand prepares deploy.Options and invokes the deployment runner.
type BootstrapCommand struct {
	RunBootstrap      func(context.Context, deploy.Options) error
	GenerateClusterID func() (string, error)
	GenerateWorkerID  func() (string, error)
	GenerateAPIKey    func() (string, error)
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

	clusterGen := c.GenerateClusterID
	if clusterGen == nil {
		clusterGen = func() (string, error) {
			return gonanoid.Generate(DefaultClusterIDAlphabet, DefaultClusterIDLength)
		}
	}
	workerGen := c.GenerateWorkerID
	if workerGen == nil {
		workerGen = func() (string, error) {
			return gonanoid.Generate(DefaultWorkerIDAlphabet, DefaultWorkerIDLength)
		}
	}
	apiGen := c.GenerateAPIKey
	if apiGen == nil {
		apiGen = func() (string, error) {
			return gonanoid.Generate(DefaultAPIKeyAlphabet, DefaultAPIKeyLength)
		}
	}

	workstationOS := strings.TrimSpace(cfg.WorkstationOS)
	if workstationOS == "" {
		workstationOS = runtime.GOOS
	}

	clusterID, err := clusterGen()
	if err != nil {
		return fmt.Errorf("generate cluster identifier: %w", err)
	}

	opts := deploy.Options{}
	opts.ClusterID = clusterID
	opts.User = strings.TrimSpace(cfg.User)
	opts.Address = strings.TrimSpace(cfg.Address)
	opts.ControlPlaneURL = strings.TrimSpace(cfg.ControlPlaneURL)
	opts.WorkstationOS = workstationOS

	nodeID, err := workerGen()
	if err != nil {
		return fmt.Errorf("generate node identifier: %w", err)
	}
	opts.InitialBeacons = []string{nodeID}
	opts.InitialWorkers = []string{nodeID}

	manualBeaconURL := strings.TrimSpace(cfg.BeaconURL)
	if manualBeaconURL != "" {
		opts.BeaconURL = manualBeaconURL
	} else {
		opts.BeaconURL = fmt.Sprintf("https://%s.%s%s", nodeID, clusterID, DefaultDomainSuffix)
	}

	apiKey, err := apiGen()
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}
	opts.APIKey = apiKey

	opts.Host = clusterID + DefaultDomainSuffix

	connectHost := opts.Address
	if connectHost == "" {
		connectHost = opts.Host
	}
	if connectHost != "" {
		etcdHost := connectHost
		if strings.Contains(etcdHost, ":") && !strings.Contains(etcdHost, "]") && !strings.Contains(etcdHost, "[") {
			etcdHost = "[" + etcdHost + "]"
		}
		opts.EtcdEndpoints = []string{fmt.Sprintf("http://%s:2379", etcdHost)}
	}

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

	adminPath := strings.TrimSpace(cfg.AdminAuthorizedKeysPath)
	if adminPath == "" {
		return ErrAdminAuthorizedKeysRequired
	}
	adminKeys, err := readAuthorizedKeysFile(ExpandPath(adminPath))
	if err != nil {
		return fmt.Errorf("load admin authorized keys: %w", err)
	}
	opts.AdminAuthorizedKeys = adminKeys

	userPath := strings.TrimSpace(cfg.UserAuthorizedKeysPath)
	if userPath == "" {
		return ErrUserAuthorizedKeysRequired
	}
	userKeys, err := readAuthorizedKeysFile(ExpandPath(userPath))
	if err != nil {
		return fmt.Errorf("load user authorized keys: %w", err)
	}
	opts.UserAuthorizedKeys = userKeys

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

	if opts.BeaconURL == "" {
		return ErrBeaconURLRequired
	}
	if opts.APIKey == "" {
		return ErrAPIKeyRequired
	}
	if len(opts.InitialBeacons) == 0 {
		return ErrInitialBeaconIDMissing
	}

	if ctx == nil {
		ctx = context.Background()
	}
	return runner(ctx, opts)
}

// readAuthorizedKeysFile loads authorized keys from the provided path, skipping blanks and comments.
func readAuthorizedKeysFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read authorized keys %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	keys := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		keys = append(keys, trimmed)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("authorized keys file %s has no keys", path)
	}
	return keys, nil
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
	return "", errors.New("ploy deploy bootstrap: ployd binary not found alongside CLI; provide --ployd-binary")
}
