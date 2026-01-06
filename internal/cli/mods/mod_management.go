// Package mods provides CLI client implementations for Mods operations.
// This file implements mod project management commands (add, list, remove, archive, unarchive).
//
// These commands call the server endpoints:
// - POST /v1/mods (create mod)
// - GET /v1/mods (list mods)
// - DELETE /v1/mods/{mod_id} (delete mod)
// - PATCH /v1/mods/{mod_id}/archive (archive mod)
// - PATCH /v1/mods/{mod_id}/unarchive (unarchive mod)
//
// Per roadmap/v1/cli.md:24-50, these commands implement the mod management surfaces.
package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ModSummary represents a mod project returned by the server.
// Matches the server response shape from internal/server/handlers/mods.go.
type ModSummary struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	SpecID    *string `json:"spec_id,omitempty"`
	CreatedBy *string `json:"created_by,omitempty"`
	Archived  bool    `json:"archived"`
	CreatedAt string  `json:"created_at"`
}

// AddModCommand creates a new mod project.
// Endpoint: POST /v1/mods
// Per roadmap/v1/cli.md:26-31, this creates a mod with unique name and optional spec.
type AddModCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Name      string           // Required: unique mod name.
	Spec      *json.RawMessage // Optional: initial spec (creates spec row and sets mods.spec_id).
	CreatedBy *string          // Optional: creator identifier.
}

// AddModResult contains the response from creating a mod.
type AddModResult struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	SpecID    *string `json:"spec_id,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// Run executes POST /v1/mods to create a mod project.
func (c AddModCommand) Run(ctx context.Context) (AddModResult, error) {
	if c.Client == nil {
		return AddModResult{}, fmt.Errorf("mod add: http client required")
	}
	if c.BaseURL == nil {
		return AddModResult{}, fmt.Errorf("mod add: base url required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return AddModResult{}, fmt.Errorf("mod add: name is required")
	}

	// Build request payload per roadmap/v1/api.md:15-23.
	req := struct {
		Name      string           `json:"name"`
		Spec      *json.RawMessage `json:"spec,omitempty"`
		CreatedBy *string          `json:"created_by,omitempty"`
	}{
		Name:      strings.TrimSpace(c.Name),
		Spec:      c.Spec,
		CreatedBy: c.CreatedBy,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return AddModResult{}, fmt.Errorf("mod add: marshal request: %w", err)
	}

	// POST /v1/mods to create the mod.
	endpoint := c.BaseURL.JoinPath("/v1/mods")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return AddModResult{}, fmt.Errorf("mod add: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return AddModResult{}, fmt.Errorf("mod add: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 201 Created response.
	if resp.StatusCode == http.StatusCreated {
		var result AddModResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return AddModResult{}, fmt.Errorf("mod add: decode response: %w", err)
		}
		return result, nil
	}

	// Non-success: read error body and return error.
	return AddModResult{}, decodeHTTPError(resp, "mod add")
}

// ListModsCommand lists mod projects with optional filters.
// Endpoint: GET /v1/mods
// Per roadmap/v1/cli.md:33-35, this lists mods: ID, NAME, CREATED_AT, ARCHIVED_AT.
type ListModsCommand struct {
	Client        *http.Client
	BaseURL       *url.URL
	Limit         int32   // Max results to return (default 50, max 100).
	Offset        int32   // Number of results to skip.
	NameSubstring *string // Optional: filter by name substring.
	Archived      *bool   // Optional: filter by archived status.
	RepoURL       *string // Optional: filter by repo URL in repo set.
}

// Run executes GET /v1/mods to list mods with pagination and filters.
func (c ListModsCommand) Run(ctx context.Context) ([]ModSummary, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("mod list: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("mod list: base url required")
	}

	// Build endpoint with query params.
	endpoint := c.BaseURL.JoinPath("/v1/mods")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if c.NameSubstring != nil && *c.NameSubstring != "" {
		q.Set("name_substring", *c.NameSubstring)
	}
	if c.Archived != nil {
		q.Set("archived", fmt.Sprintf("%t", *c.Archived))
	}
	if c.RepoURL != nil && *c.RepoURL != "" {
		q.Set("repo_url", *c.RepoURL)
	}
	endpoint.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("mod list: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mod list: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "mod list")
	}

	// Response structure: {"mods": [...]}
	var result struct {
		Mods []ModSummary `json:"mods"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mod list: decode response: %w", err)
	}

	return result.Mods, nil
}

// RemoveModCommand deletes a mod project.
// Endpoint: DELETE /v1/mods/{mod_id}
// Per roadmap/v1/cli.md:37-40, this refuses deletion if the mod has any runs.
type RemoveModCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModID   string // Required: mod ID or name to delete.
}

// Run executes DELETE /v1/mods/{mod_id} to delete a mod.
func (c RemoveModCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return fmt.Errorf("mod remove: http client required")
	}
	if c.BaseURL == nil {
		return fmt.Errorf("mod remove: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return fmt.Errorf("mod remove: mod id is required")
	}

	// DELETE /v1/mods/{mod_id}
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("mod remove: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mod remove: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 204 No Content indicates success.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return decodeHTTPError(resp, "mod remove")
}

// ArchiveModCommand archives a mod project.
// Endpoint: PATCH /v1/mods/{mod_id}/archive
// Per roadmap/v1/cli.md:42-45, this refuses archival if the mod has running jobs.
type ArchiveModCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModID   string // Required: mod ID or name to archive.
}

// ArchiveModResult contains the response from archiving a mod.
type ArchiveModResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

// Run executes PATCH /v1/mods/{mod_id}/archive to archive a mod.
func (c ArchiveModCommand) Run(ctx context.Context) (ArchiveModResult, error) {
	if c.Client == nil {
		return ArchiveModResult{}, fmt.Errorf("mod archive: http client required")
	}
	if c.BaseURL == nil {
		return ArchiveModResult{}, fmt.Errorf("mod archive: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return ArchiveModResult{}, fmt.Errorf("mod archive: mod id is required")
	}

	// PATCH /v1/mods/{mod_id}/archive
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)), "archive")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint.String(), nil)
	if err != nil {
		return ArchiveModResult{}, fmt.Errorf("mod archive: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return ArchiveModResult{}, fmt.Errorf("mod archive: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		var result ArchiveModResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return ArchiveModResult{}, fmt.Errorf("mod archive: decode response: %w", err)
		}
		return result, nil
	}

	return ArchiveModResult{}, decodeHTTPError(resp, "mod archive")
}

// UnarchiveModCommand unarchives a mod project.
// Endpoint: PATCH /v1/mods/{mod_id}/unarchive
// Per roadmap/v1/cli.md:47-49.
type UnarchiveModCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModID   string // Required: mod ID or name to unarchive.
}

// UnarchiveModResult contains the response from unarchiving a mod.
type UnarchiveModResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

// Run executes PATCH /v1/mods/{mod_id}/unarchive to unarchive a mod.
func (c UnarchiveModCommand) Run(ctx context.Context) (UnarchiveModResult, error) {
	if c.Client == nil {
		return UnarchiveModResult{}, fmt.Errorf("mod unarchive: http client required")
	}
	if c.BaseURL == nil {
		return UnarchiveModResult{}, fmt.Errorf("mod unarchive: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return UnarchiveModResult{}, fmt.Errorf("mod unarchive: mod id is required")
	}

	// PATCH /v1/mods/{mod_id}/unarchive
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)), "unarchive")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint.String(), nil)
	if err != nil {
		return UnarchiveModResult{}, fmt.Errorf("mod unarchive: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return UnarchiveModResult{}, fmt.Errorf("mod unarchive: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		var result UnarchiveModResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return UnarchiveModResult{}, fmt.Errorf("mod unarchive: decode response: %w", err)
		}
		return result, nil
	}

	return UnarchiveModResult{}, decodeHTTPError(resp, "mod unarchive")
}

// SetModSpecCommand creates a new spec row and updates mods.spec_id.
// Endpoint: POST /v1/mods/{mod_id}/specs
// Per roadmap/v1/cli.md:53-60, this sets the mod's current spec.
type SetModSpecCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	ModID     string          // Required: mod ID or name.
	Spec      json.RawMessage // Required: spec content (YAML/JSON parsed to JSON).
	Name      *string         // Optional: spec name.
	CreatedBy *string         // Optional: creator identifier.
}

// SetModSpecResult contains the response from setting a mod spec.
type SetModSpecResult struct {
	ID        string `json:"id"` // spec_id
	CreatedAt string `json:"created_at"`
}

// Run executes POST /v1/mods/{mod_id}/specs to set the mod's spec.
func (c SetModSpecCommand) Run(ctx context.Context) (SetModSpecResult, error) {
	if c.Client == nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: http client required")
	}
	if c.BaseURL == nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: base url required")
	}
	if strings.TrimSpace(c.ModID) == "" {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: mod id is required")
	}
	if len(c.Spec) == 0 {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: spec is required")
	}

	// Build request payload per roadmap/v1/api.md:143-147.
	req := struct {
		Name      string          `json:"name,omitempty"`
		Spec      json.RawMessage `json:"spec"`
		CreatedBy *string         `json:"created_by,omitempty"`
	}{
		Spec:      c.Spec,
		CreatedBy: c.CreatedBy,
	}
	if c.Name != nil {
		req.Name = *c.Name
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: marshal request: %w", err)
	}

	// POST /v1/mods/{mod_id}/specs
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(strings.TrimSpace(c.ModID)), "specs")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 201 Created response.
	if resp.StatusCode == http.StatusCreated {
		var result SetModSpecResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return SetModSpecResult{}, fmt.Errorf("mod spec set: decode response: %w", err)
		}
		return result, nil
	}

	return SetModSpecResult{}, decodeHTTPError(resp, "mod spec set")
}

// ResolveModByNameCommand attempts to resolve a mod by name (if not a valid ID format).
// This is used to support name/ID resolution per roadmap/v1/cli.md:169-170.
// It lists mods and finds an exact name match.
type ResolveModByNameCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModRef  string // Mod reference (could be ID or name).
}

// Run attempts to resolve a mod ID from a name reference.
// Returns the mod ID if found (either as-is if it looks like an ID, or resolved from name).
func (c ResolveModByNameCommand) Run(ctx context.Context) (string, error) {
	if c.Client == nil {
		return "", fmt.Errorf("resolve mod: http client required")
	}
	if c.BaseURL == nil {
		return "", fmt.Errorf("resolve mod: base url required")
	}
	ref := strings.TrimSpace(c.ModRef)
	if ref == "" {
		return "", fmt.Errorf("resolve mod: mod reference is required")
	}

	// If it looks like a UUID-style ID (contains hyphens and is 36 chars), return as-is.
	// This is a heuristic; the server will validate the actual ID.
	if len(ref) == 36 && strings.Count(ref, "-") == 4 {
		return ref, nil
	}

	// Otherwise, try to find by name using the list endpoint with name filter.
	// Per roadmap/v1/cli.md:170, prefer exact name match.
	listCmd := ListModsCommand{
		Client:        c.Client,
		BaseURL:       c.BaseURL,
		Limit:         100,
		NameSubstring: &ref,
	}

	mods, err := listCmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve mod: %w", err)
	}

	// Find exact name match.
	var matches []ModSummary
	for _, mod := range mods {
		if mod.Name == ref {
			matches = append(matches, mod)
		}
	}

	switch len(matches) {
	case 0:
		// No exact match; the reference might be an ID, pass it through.
		return ref, nil
	case 1:
		return matches[0].ID, nil
	default:
		// Multiple exact matches should not happen (name is unique), but handle gracefully.
		return "", fmt.Errorf("resolve mod: multiple mods found with name %q", ref)
	}
}
