package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func submitSingleRepoRun(ctx context.Context, base *url.URL, httpClient *http.Client, request domainapi.RunSubmitRequest) (domaintypes.RunID, domaintypes.MigID, error) {
	if base == nil {
		return "", "", fmt.Errorf("run submit: base url required")
	}
	if httpClient == nil {
		return "", "", fmt.Errorf("run submit: http client required")
	}

	endpoint := base.JoinPath("v1", "runs")
	payload, err := json.Marshal(request)
	if err != nil {
		return "", "", fmt.Errorf("run submit: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("run submit: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("run submit: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &apiErr); err == nil {
			if msg := strings.TrimSpace(apiErr.Error); msg != "" {
				return "", "", fmt.Errorf("run submit: %s", msg)
			}
		}
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return "", "", fmt.Errorf("run submit: %s", msg)
	}

	var created struct {
		RunID  domaintypes.RunID  `json:"run_id"`
		MigID  domaintypes.MigID  `json:"mig_id"`
		SpecID domaintypes.SpecID `json:"spec_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", "", fmt.Errorf("run submit: decode response: %w", err)
	}
	if created.RunID.IsZero() {
		return "", "", fmt.Errorf("run submit: empty run_id in response")
	}
	if created.MigID.IsZero() {
		return "", "", fmt.Errorf("run submit: empty mig_id in response")
	}
	return created.RunID, created.MigID, nil
}
