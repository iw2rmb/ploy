package tunnel

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/cache"
	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

var (
	managerOnce sync.Once
	managerInst *sshtransport.Manager
	managerErr  error

	cacheOnce sync.Once
	cacheInst *cache.Cache
	cacheErr  error
)

// Manager returns the shared SSH tunnel manager instance.
func Manager() (*sshtransport.Manager, error) {
	managerOnce.Do(func() {
		c, err := ensureCache()
		if err != nil {
			managerErr = err
			return
		}
		cfg := sshtransport.Config{
			ControlSocketDir: controlSocketDir(),
			LocalAddress:     "127.0.0.1",
			Cache:            c,
		}
		inst, err := sshtransport.NewManager(cfg)
		if err != nil {
			managerErr = err
			return
		}
		managerInst = inst
	})
	return managerInst, managerErr
}

// Cache exposes the persisted assignment cache.
func Cache() (*cache.Cache, error) {
	return ensureCache()
}

// Nodes returns the cached node snapshot known to the tunnel cache.
func Nodes() []sshtransport.Node {
	store, err := ensureCache()
	if err != nil || store == nil {
		return nil
	}
	return store.Nodes()
}

func ensureCache() (*cache.Cache, error) {
	cacheOnce.Do(func() {
		path := cachePath()
		inst, err := cache.New(path)
		if err != nil {
			cacheErr = err
			return
		}
		cacheInst = inst
	})
	return cacheInst, cacheErr
}

// SetNodes configures the shared manager with the provided node snapshot.
func SetNodes(nodes []sshtransport.Node) error {
	manager, err := Manager()
	if err != nil {
		return err
	}
	return manager.SetNodes(nodes)
}

// NodesFromGridStatus converts grid metadata into SSH tunnel nodes.
// Former helpers that transformed external grid metadata into SSH nodes were removed;
// callers should compute sshtransport.Node values from their own control-plane discovery.

// EnsureFallbackNode registers a single fallback node derived from the base URL when discovery metadata is unavailable.
func EnsureFallbackNode(base *url.URL) error {
	if base == nil {
		return errors.New("tunnel: base url required")
	}
	store, err := ensureCache()
	if err != nil {
		return err
	}
	if existing := store.Nodes(); len(existing) > 0 {
		return nil
	}
	host := base.Hostname()
	if host == "" {
		return fmt.Errorf("tunnel: invalid base host %q", base.Host)
	}
	port := resolvePort(base)
	node := sshtransport.Node{
		ID:           "fallback-" + host,
		Address:      host,
		SSHPort:      22,
		APIPort:      port,
		User:         defaultSSHUser(),
		IdentityFile: defaultIdentityFile(),
	}
	return SetNodes([]sshtransport.Node{node})
}

// AttachHTTP configures the client's transport to dial via the shared tunnel manager.
func AttachHTTP(client *http.Client) error {
	if client == nil {
		return errors.New("tunnel: http client required")
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	manager, err := Manager()
	if err != nil {
		return err
	}

	if !manager.HasTargets() {
		return nil
	}

	switch transport := client.Transport.(type) {
	case *http.Transport:
		transport.DialContext = manager.DialContext
		return nil
	case interface{ BaseTransport() *http.Transport }:
		base := transport.BaseTransport()
		if base == nil {
			return errors.New("tunnel: base transport unavailable")
		}
		base.DialContext = manager.DialContext
		return nil
	default:
		return fmt.Errorf("tunnel: unsupported transport type %T", client.Transport)
	}
}

// RememberJob records job-to-node assignments in the shared cache.
func RememberJob(jobID, nodeID string, at time.Time) error {
	store, err := ensureCache()
	if err != nil {
		return err
	}
	return store.RememberJob(jobID, nodeID, at)
}

// LookupJob returns the cached node assignment for the supplied job identifier.
func LookupJob(jobID string) (string, bool) {
	store, err := ensureCache()
	if err != nil {
		return "", false
	}
	return store.LookupJob(jobID)
}

// Deprecated URL parsing helpers removed; resolvePort() covers active use cases.

func resolvePort(base *url.URL) int {
	if base == nil {
		return 443
	}
	port := base.Port()
	if port != "" {
		value, err := net.LookupPort(base.Scheme, port)
		if err == nil {
			return value
		}
	}
	switch strings.ToLower(base.Scheme) {
	case "https":
		return 443
	case "http":
		return 80
	default:
		return 443
	}
}

func defaultSSHUser() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_SSH_USER")); value != "" {
		return value
	}
	return "ploy"
}

func defaultIdentityFile() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_SSH_IDENTITY")); value != "" {
		return expandPath(value)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "id_rsa")
}

func controlSocketDir() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_SSH_SOCKET_DIR")); value != "" {
		return expandPath(value)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ploy-tunnels")
	}
	return filepath.Join(home, ".ploy", "tunnels")
}

func cachePath() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_CACHE_HOME")); value != "" {
		return filepath.Join(expandPath(value), "controlplane.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return filepath.Join(os.TempDir(), "ploy-controlplane.json")
		}
		return filepath.Join(home, ".ploy", "cache", "controlplane.json")
	}
	return filepath.Join(configDir, "ploy", "cache", "controlplane.json")
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}
