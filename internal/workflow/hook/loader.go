package hook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const defaultHTTPTimeout = 15 * time.Second

// Loader resolves and loads hook manifests declared from root-level mig spec hooks.
type Loader struct {
	HTTPClient *http.Client
}

type resolvedSource struct {
	Canonical string
	Remote    bool
}

// LoadFromMigSpec resolves and loads hook manifests from spec.Hooks.
func LoadFromMigSpec(spec contracts.MigSpec, specRoot string) ([]Spec, error) {
	return NewLoader(nil).LoadFromMigSpec(spec, specRoot)
}

// NewLoader creates a loader with optional custom HTTP client.
func NewLoader(client *http.Client) Loader {
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return Loader{HTTPClient: client}
}

// LoadFromMigSpec resolves direct files, direct URLs, and recursive hook.yaml files in directories.
func (l Loader) LoadFromMigSpec(spec contracts.MigSpec, specRoot string) ([]Spec, error) {
	var loaded []Spec
	var errs []error
	seenSources := make(map[string]struct{})

	for i, raw := range spec.Hooks {
		entry := strings.TrimSpace(raw)
		resolved, err := resolveHookSource(entry, specRoot)
		if err != nil {
			errs = append(errs, fmt.Errorf("hooks[%d] %q: %w", i, raw, err))
			continue
		}
		for _, src := range resolved {
			if _, dup := seenSources[src.Canonical]; dup {
				errs = append(errs, fmt.Errorf("hooks[%d] %q: duplicate resolved hook manifest %q", i, raw, src.Canonical))
				continue
			}
			seenSources[src.Canonical] = struct{}{}

			specDoc, loadErr := l.loadSpecFromSource(src)
			if loadErr != nil {
				errs = append(errs, fmt.Errorf("hooks[%d] %q: %w", i, raw, loadErr))
				continue
			}
			loaded = append(loaded, specDoc)
		}
	}

	if len(errs) > 0 {
		return nil, multiError{errs: errs}
	}
	return loaded, nil
}

func (l Loader) loadSpecFromSource(source resolvedSource) (Spec, error) {
	body, err := l.readSourceData(source)
	if err != nil {
		return Spec{}, err
	}
	return LoadSpecYAML(body, source.Canonical)
}

func (l Loader) readSourceData(source resolvedSource) ([]byte, error) {
	if !source.Remote {
		body, err := os.ReadFile(source.Canonical)
		if err != nil {
			return nil, fmt.Errorf("read hook manifest %s: %w", source.Canonical, err)
		}
		return body, nil
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, source.Canonical, nil)
	if err != nil {
		return nil, fmt.Errorf("build hook manifest request %s: %w", source.Canonical, err)
	}
	resp, err := l.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download hook manifest %s: %w", source.Canonical, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download hook manifest %s: unexpected status %d", source.Canonical, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read hook manifest %s: %w", source.Canonical, err)
	}
	return body, nil
}

func resolveHookSource(source string, specRoot string) ([]resolvedSource, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("empty hook source")
	}
	if remote, ok := parseRemoteSource(source); ok {
		return []resolvedSource{{Canonical: remote, Remote: true}}, nil
	}

	local := source
	if !filepath.IsAbs(local) {
		local = filepath.Join(specRoot, local)
	}
	local = filepath.Clean(local)
	info, err := os.Stat(local)
	if err != nil {
		return nil, fmt.Errorf("stat hook source %q: %w", local, err)
	}
	if !info.IsDir() {
		return []resolvedSource{{Canonical: local}}, nil
	}

	hooks, err := discoverHookManifests(local)
	if err != nil {
		return nil, err
	}
	if len(hooks) == 0 {
		return nil, fmt.Errorf("directory hook source %q: no hook.yaml files found", local)
	}

	result := make([]resolvedSource, 0, len(hooks))
	for _, path := range hooks {
		result = append(result, resolvedSource{Canonical: path})
	}
	return result, nil
}

func discoverHookManifests(root string) ([]string, error) {
	root = filepath.Clean(root)
	var hookFiles []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "hook.yaml" {
			return nil
		}
		hookFiles = append(hookFiles, filepath.Clean(path))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk hook source directory %q: %w", root, err)
	}
	sort.Strings(hookFiles)
	return hookFiles, nil
}

func parseRemoteSource(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return "", false
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

type multiError struct {
	errs []error
}

func (m multiError) Error() string {
	if len(m.errs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, err := range m.errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		_, _ = fmt.Fprintf(&b, "%d. %v", i+1, err)
	}
	return b.String()
}

func (m multiError) Unwrap() error {
	return errors.Join(m.errs...)
}
