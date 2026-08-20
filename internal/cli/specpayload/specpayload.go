package specpayload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/speccompiler"
)

type localSource struct{}

func (localSource) ResolveSpec(path string) (string, error) {
	resolved, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %s: %w", path, err)
	}
	return resolved, nil
}

func (localSource) ResolveBaseDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return os.Getwd()
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %s: %w", path, err)
	}
	return filepath.Clean(resolved), nil
}

func (localSource) ResolveReference(path, baseDir string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !filepath.IsAbs(trimmed) && strings.TrimSpace(baseDir) != "" {
		trimmed = filepath.Join(baseDir, trimmed)
	}
	resolved, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %s: %w", trimmed, err)
	}
	return filepath.Clean(resolved), nil
}

func (localSource) ResolveFileRecord(path, baseDir string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is empty")
	}
	expanded := strings.TrimSpace(os.ExpandEnv(trimmed))
	if expanded == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir for path %s: %w", expanded, err)
		}
		expanded = filepath.Join(home, expanded[2:])
	} else if !filepath.IsAbs(expanded) && strings.TrimSpace(baseDir) != "" {
		expanded = filepath.Join(baseDir, expanded)
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %s: %w", expanded, err)
	}
	return filepath.Clean(resolved), nil
}

func (localSource) ReadFile(path string) ([]byte, error)       { return common.ReadFileRooted(path) }
func (localSource) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
func (localSource) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localSource) WorkingDir() (string, error)                { return os.Getwd() }

type httpBundleStore struct {
	base   *url.URL
	client *http.Client
}

func (s httpBundleStore) Ensure(ctx context.Context, cid string, archive []byte) (string, error) {
	if s.base == nil {
		return "", fmt.Errorf("file-backed records found but no server base URL available for upload")
	}
	if s.client == nil {
		return "", fmt.Errorf("file-backed records found but no HTTP client available for upload")
	}
	bundleID, exists, err := s.probe(ctx, cid)
	if err != nil {
		return "", fmt.Errorf("probe: %w", err)
	}
	if exists && bundleID != "" {
		return bundleID, nil
	}
	return s.upload(ctx, archive)
}

func (s httpBundleStore) probe(ctx context.Context, cid string) (string, bool, error) {
	endpoint := s.base.JoinPath("v1", "spec-bundles")
	query := endpoint.Query()
	query.Set("cid", cid)
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("spec-bundle probe: build request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("spec-bundle probe: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Header.Get("X-Bundle-ID"), true, nil
	case http.StatusNotFound:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("spec-bundle probe: unexpected status %s", resp.Status)
	}
}

func (s httpBundleStore) upload(ctx context.Context, archive []byte) (string, error) {
	endpoint := s.base.JoinPath("v1", "spec-bundles")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(archive))
	if err != nil {
		return "", fmt.Errorf("spec-bundle upload: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("spec-bundle upload: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
			return "", fmt.Errorf("spec-bundle upload: server error: %s", apiErr.Error)
		}
		return "", fmt.Errorf("spec-bundle upload: unexpected status %s", resp.Status)
	}
	var response struct {
		BundleID string `json:"bundle_id"`
		CID      string `json:"cid"`
		Digest   string `json:"digest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("spec-bundle upload: decode response: %w", err)
	}
	if response.BundleID == "" {
		return "", fmt.Errorf("spec-bundle upload: empty bundle_id in response")
	}
	if response.CID == "" {
		return "", fmt.Errorf("spec-bundle upload: empty cid in response")
	}
	if response.Digest == "" {
		return "", fmt.Errorf("spec-bundle upload: empty digest in response")
	}
	return response.BundleID, nil
}

func newCompiler(base *url.URL, client *http.Client) (*speccompiler.Compiler, error) {
	return speccompiler.New(speccompiler.Options{
		Source:       localSource{},
		LookupEnv:    os.LookupEnv,
		ApplyOverlay: applyConfigOverlay,
		Bundles:      httpBundleStore{base: base, client: client},
	})
}

func applyConfigOverlay(spec map[string]any) error {
	overlay, err := cliconfig.LoadOverlay()
	if err != nil {
		return fmt.Errorf("config overlay: %w", err)
	}
	if overlay.Defaults == nil || overlay.Defaults.Job == nil {
		return nil
	}
	migConfig := overlay.JobSection("mig")
	if migConfig != nil && len(migConfig.Envs) > 0 {
		cliconfig.MergeJobConfigIntoSpec(spec, &cliconfig.JobConfig{Envs: migConfig.Envs})
	}
	if steps, ok := spec["steps"].([]any); ok {
		for _, rawStep := range steps {
			if step, ok := rawStep.(map[string]any); ok {
				cliconfig.MergeJobConfigIntoSpec(step, migConfig)
			}
		}
	}
	return nil
}

func Normalize(ctx context.Context, base *url.URL, client *http.Client, data []byte, specBaseDir string) (json.RawMessage, error) {
	compiler, err := newCompiler(base, client)
	if err != nil {
		return nil, err
	}
	return compiler.Normalize(ctx, data, specBaseDir)
}

func ValidateLocalFile(path string) (json.RawMessage, error) {
	compiler, err := newCompiler(nil, nil)
	if err != nil {
		return nil, err
	}
	return compiler.ValidateFile(path)
}

func BuildSelected(ctx context.Context, base *url.URL, client *http.Client, specFile, stepSelector string, migEnvs []string, migImage string, retain bool, migCommand string) ([]byte, error) {
	_ = retain
	compiler, err := newCompiler(base, client)
	if err != nil {
		return nil, err
	}
	return compiler.BuildSelected(ctx, specFile, stepSelector, speccompiler.Overrides{
		Envs: migEnvs, Image: migImage, Command: migCommand,
	})
}

func Load(ctx context.Context, base *url.URL, client *http.Client, path string) (json.RawMessage, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	} else {
		data, err = common.ReadFileRooted(path)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", path, err)
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("spec is empty")
	}
	baseDir := filepath.Dir(path)
	if path == "-" {
		baseDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve working directory for stdin spec: %w", err)
		}
	}
	return Normalize(ctx, base, client, data, baseDir)
}
