package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	"github.com/iw2rmb/ploy/internal/cli/specpayload"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

var namedSpecSHAPrefixRE = regexp.MustCompile(`^[0-9a-f]{8,40}$`)

type runSubmitSpecPayload struct {
	Spec        json.RawMessage
	SpecID      domaintypes.SpecID
	DisplayName string
}

func resolveRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, specArg string) (runSubmitSpecPayload, error) {
	specArg = strings.TrimSpace(specArg)
	if specArg == "" {
		return runSubmitSpecPayload{}, errors.New("spec path required")
	}
	if info, err := os.Stat(specArg); err == nil {
		specPath, err := normalizeRunSpecPath(specArg, info)
		if err != nil {
			return runSubmitSpecPayload{}, err
		}
		spec, err := buildRunSubmitSpecPayload(ctx, base, client, specPath, "")
		return runSubmitSpecPayload{Spec: spec}, err
	}

	localPath, stepSelector, ok, err := splitLocalRunSpecSelector(specArg)
	if err != nil {
		return runSubmitSpecPayload{}, err
	}
	if ok {
		specPath, err := resolveRunSpecPath(localPath)
		if err != nil {
			return runSubmitSpecPayload{}, err
		}
		spec, err := buildRunSubmitSpecPayload(ctx, base, client, specPath, stepSelector)
		return runSubmitSpecPayload{Spec: spec}, err
	}

	if pathExplicitlyLocal(specArg) {
		specPath, err := resolveRunSpecPath(specArg)
		if err != nil {
			return runSubmitSpecPayload{}, err
		}
		spec, err := buildRunSubmitSpecPayload(ctx, base, client, specPath, "")
		return runSubmitSpecPayload{Spec: spec}, err
	}

	return resolveNamedRunSubmitSpecPayload(ctx, base, client, specArg)
}

func splitLocalRunSpecSelector(specArg string) (string, string, bool, error) {
	if _, err := os.Stat(specArg); err == nil {
		return specArg, "", true, nil
	}
	idx := strings.LastIndex(specArg, ":")
	if idx < 0 {
		return "", "", false, nil
	}
	specPath := strings.TrimSpace(specArg[:idx])
	stepSelector := strings.TrimSpace(specArg[idx+1:])
	if !pathExplicitlyLocal(specPath) {
		if _, err := os.Stat(specPath); err != nil {
			return "", "", false, nil
		}
	}
	if specPath == "" {
		return "", "", true, errors.New("spec path required")
	}
	if stepSelector == "" {
		return "", "", true, errors.New("step name required")
	}
	return specPath, stepSelector, true, nil
}

func pathExplicitlyLocal(path string) bool {
	return strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || filepath.IsAbs(path)
}

func resolveRunSpecPath(specPath string) (string, error) {
	specPath = strings.TrimSpace(specPath)
	if specPath == "" {
		return "", errors.New("spec path required")
	}
	info, err := os.Stat(specPath)
	if err != nil {
		return "", fmt.Errorf("load spec: %w", err)
	}
	return normalizeRunSpecPath(specPath, info)
}

func normalizeRunSpecPath(specPath string, info os.FileInfo) (string, error) {
	if info.IsDir() {
		specPath = filepath.Join(specPath, "mig.yaml")
		if _, err := os.Stat(specPath); err != nil {
			return "", fmt.Errorf("load spec: %w", err)
		}
	}
	return specPath, nil
}

func resolveNamedRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, selector string) (runSubmitSpecPayload, error) {
	if base == nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: base url required")
	}
	if client == nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: http client required")
	}

	selector, shaPrefix, err := splitNamedSpecVersionSelector(selector)
	if err != nil {
		return runSubmitSpecPayload{}, err
	}
	endpoint := base.JoinPath("v1", "specs", "resolve")
	query := endpoint.Query()
	query.Set("selector", selector)
	if shaPrefix != "" {
		query.Set("sha", shaPrefix)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: named spec not found: %s", selector)
	case http.StatusBadRequest, http.StatusConflict:
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: %s", httpx.ReadErrorMessage(resp.Body, resp.Status, httpx.MaxErrorBodyBytes))
	default:
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: %s", httpx.ReadErrorMessage(resp.Body, resp.Status, httpx.MaxErrorBodyBytes))
	}

	var resolved domainapi.NamedSpecResolveResponse
	if err := httpx.DecodeResponseJSON(resp.Body, &resolved, httpx.MaxJSONBodyBytes); err != nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: decode response: %w", err)
	}
	if len(resolved.Spec) == 0 {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: empty spec in response")
	}
	return runSubmitSpecPayload{
		Spec:        resolved.Spec,
		SpecID:      domaintypes.SpecID(strings.TrimSpace(resolved.ID)),
		DisplayName: namedSpecDisplayName(resolved),
	}, nil
}

func splitNamedSpecVersionSelector(selector string) (string, string, error) {
	selector = strings.TrimSpace(selector)
	idx := strings.LastIndex(selector, "@")
	if idx < 0 {
		return selector, "", nil
	}
	base := strings.TrimSpace(selector[:idx])
	shaPrefix := strings.TrimSpace(selector[idx+1:])
	if base == "" || shaPrefix == "" {
		return "", "", fmt.Errorf("run submit: invalid named spec selector: %s", selector)
	}
	if !namedSpecSHAPrefixRE.MatchString(shaPrefix) {
		return "", "", fmt.Errorf("run submit: sha must be a lowercase 8-40 character hex prefix")
	}
	return base, shaPrefix, nil
}

func namedSpecDisplayName(resolved domainapi.NamedSpecResolveResponse) string {
	domain := strings.Trim(strings.TrimSpace(resolved.Source.Domain), "/")
	repo := strings.Trim(strings.TrimSpace(resolved.Source.Repo), "/")
	name := strings.TrimSpace(resolved.Name)
	if domain == "" || repo == "" || name == "" {
		return ""
	}
	return domain + "/" + repo + ":" + name
}

func buildRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, specPath string, stepSelector string) (json.RawMessage, error) {
	specPayload, err := specpayload.BuildSelected(
		ctx,
		base,
		client,
		specPath,
		stepSelector,
		nil,
		"",
		false,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	if len(specPayload) == 0 {
		return nil, fmt.Errorf("load spec: spec is empty")
	}
	return specPayload, nil
}
