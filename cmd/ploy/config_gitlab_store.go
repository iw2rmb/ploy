package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	gitlabcfg "github.com/iw2rmb/ploy/internal/config/gitlab"
)

func closeGitlabStore(store gitlabStore) {
	if closer, ok := store.(gitlabStoreCloser); ok {
		_ = closer.Close()
	}
}

type httpGitlabConfigStore struct {
	base *url.URL
	http *http.Client
}

func newHTTPGitlabConfigStore(base *url.URL, httpClient *http.Client) *httpGitlabConfigStore {
	clone := *base
	return &httpGitlabConfigStore{base: &clone, http: httpClient}
}

func (s *httpGitlabConfigStore) Close() error { return nil }

func (s *httpGitlabConfigStore) Load(ctx context.Context) (gitlabcfg.Config, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint("/v1/config/gitlab"), nil)
	if err != nil {
		return gitlabcfg.Config{}, 0, fmt.Errorf("build gitlab config request: %w", err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return gitlabcfg.Config{}, 0, fmt.Errorf("fetch gitlab config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return gitlabcfg.Config{}, 0, nil
	}
	if resp.StatusCode != http.StatusOK {
		return gitlabcfg.Config{}, 0, controlPlaneHTTPError(resp)
	}

	var payload struct {
		Config   gitlabcfg.Config `json:"config"`
		Revision int64            `json:"revision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return gitlabcfg.Config{}, 0, fmt.Errorf("decode gitlab config response: %w", err)
	}
	return payload.Config, payload.Revision, nil
}

func (s *httpGitlabConfigStore) Save(ctx context.Context, cfg gitlabcfg.Config) (int64, error) {
	_, revision, err := s.Load(ctx)
	if err != nil {
		return 0, err
	}

	request := map[string]any{
		"revision": revision,
		"config":   cfg,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("marshal gitlab config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.endpoint("/v1/config/gitlab"), bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build gitlab config update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("update gitlab config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, controlPlaneHTTPError(resp)
	}

	var payload struct {
		Revision int64 `json:"revision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode gitlab config update response: %w", err)
	}
	return payload.Revision, nil
}

func (s *httpGitlabConfigStore) endpoint(path string) string {
	u := *s.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawQuery = ""
	return u.String()
}
