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
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/specpayload"
)

type runSubmitSpecPayload struct {
	Spec         json.RawMessage
	SpecSelector string
	DisplayName  string
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

	return runSubmitSpecPayload{SpecSelector: specArg, DisplayName: specArg}, nil
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
