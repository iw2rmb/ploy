// Package mods provides CLI client implementations for Mods operations.
// This file implements mod project management commands (add, list, remove, archive, unarchive).
//
// These commands call the server endpoints:
// - POST /v1/mods (create mod)
// - GET /v1/mods (list mods)
// - DELETE /v1/mods/{mod_ref} (delete mod)
// - PATCH /v1/mods/{mod_ref}/archive (archive mod)
// - PATCH /v1/mods/{mod_ref}/unarchive (unarchive mod)
//
// These commands implement the mod management surfaces (create, list, delete, archive).
package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	"github.com/iw2rmb/ploy/internal/domain/types"
)

// ModSummary represents a mod project returned by the server.
// Matches the server response shape from internal/server/handlers/mods.go.
type ModSummary struct {
	ID        types.MigID   `json:"id"`
	Name      string        `json:"name"`
	SpecID    *types.SpecID `json:"spec_id,omitempty"`
	CreatedBy *string       `json:"created_by,omitempty"`
	Archived  bool          `json:"archived"`
	CreatedAt time.Time     `json:"created_at"`
}

// AddModCommand creates a new mod project.
// Endpoint: POST /v1/mods
// Creates a mod with unique name and optional initial spec.
type AddModCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Name      string           // Required: unique mod name.
	Spec      *json.RawMessage // Optional: initial spec (creates spec row and sets mods.spec_id).
	CreatedBy *string          // Optional: creator identifier.
}

// AddModResult contains the response from creating a mod.
type AddModResult struct {
	ID        types.MigID   `json:"id"`
	Name      string        `json:"name"`
	SpecID    *types.SpecID `json:"spec_id,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
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

	// Build request payload with name, optional spec, and optional created_by.
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
	endpoint := c.BaseURL.JoinPath("v1", "mods")
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
		if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return AddModResult{}, fmt.Errorf("mod add: decode response: %w", err)
		}
		return result, nil
	}

	// Non-success: read error body and return error.
	return AddModResult{}, decodeHTTPError(resp, "mod add")
}

// ListMigsCommand lists mod projects with optional filters.
// Endpoint: GET /v1/mods
// Returns mods with ID, NAME, CREATED_AT, ARCHIVED status.
type ListMigsCommand struct {
	Client        *http.Client
	BaseURL       *url.URL
	Limit         int32   // Max results to return (default 50, max 100).
	Offset        int32   // Number of results to skip.
	NameSubstring *string // Optional: filter by name substring.
	Archived      *bool   // Optional: filter by archived status.
	RepoURL       *string // Optional: filter by repo URL in repo set.
}

// Run executes GET /v1/mods to list mods with pagination and filters.
func (c ListMigsCommand) Run(ctx context.Context) ([]ModSummary, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("mod list: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("mod list: base url required")
	}

	// Build endpoint with query params.
	endpoint := c.BaseURL.JoinPath("v1", "mods")
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
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("mod list: decode response: %w", err)
	}

	return result.Mods, nil
}

// RemoveModCommand deletes a mod project.
// Endpoint: DELETE /v1/mods/{mod_ref}
// Refuses deletion if the mod has any runs.
type RemoveModCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  types.MigRef // Required: mod ID or name to delete.
}

// Run executes DELETE /v1/mods/{mod_ref} to delete a mod.
func (c RemoveModCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return fmt.Errorf("mod remove: http client required")
	}
	if c.BaseURL == nil {
		return fmt.Errorf("mod remove: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return fmt.Errorf("mod remove: mod ref is required")
	}

	// DELETE /v1/mods/{mod_ref}
	endpoint := c.BaseURL.JoinPath("v1", "mods", c.MigRef.String())
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

// ArchiveMigCommand archives a mod project.
// Endpoint: PATCH /v1/mods/{mod_ref}/archive
// Refuses archival if the mod has running jobs.
type ArchiveMigCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  types.MigRef // Required: mod ID or name to archive.
}

// ArchiveMigResult contains the response from archiving a mod.
type ArchiveMigResult struct {
	ID       types.MigID `json:"id"`
	Name     string      `json:"name"`
	Archived bool        `json:"archived"`
}

// Run executes PATCH /v1/mods/{mod_ref}/archive to archive a mod.
func (c ArchiveMigCommand) Run(ctx context.Context) (ArchiveMigResult, error) {
	if c.Client == nil {
		return ArchiveMigResult{}, fmt.Errorf("mod archive: http client required")
	}
	if c.BaseURL == nil {
		return ArchiveMigResult{}, fmt.Errorf("mod archive: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return ArchiveMigResult{}, fmt.Errorf("mod archive: mod ref is required")
	}

	// PATCH /v1/mods/{mod_ref}/archive
	endpoint := c.BaseURL.JoinPath("v1", "mods", c.MigRef.String(), "archive")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint.String(), nil)
	if err != nil {
		return ArchiveMigResult{}, fmt.Errorf("mod archive: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return ArchiveMigResult{}, fmt.Errorf("mod archive: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		var result ArchiveMigResult
		if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return ArchiveMigResult{}, fmt.Errorf("mod archive: decode response: %w", err)
		}
		return result, nil
	}

	return ArchiveMigResult{}, decodeHTTPError(resp, "mod archive")
}

// UnarchiveMigCommand unarchives a mod project.
// Endpoint: PATCH /v1/mods/{mod_ref}/unarchive
// Restores an archived mod to active status.
type UnarchiveMigCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  types.MigRef // Required: mod ID or name to unarchive.
}

// UnarchiveMigResult contains the response from unarchiving a mod.
type UnarchiveMigResult struct {
	ID       types.MigID `json:"id"`
	Name     string      `json:"name"`
	Archived bool        `json:"archived"`
}

// Run executes PATCH /v1/mods/{mod_ref}/unarchive to unarchive a mod.
func (c UnarchiveMigCommand) Run(ctx context.Context) (UnarchiveMigResult, error) {
	if c.Client == nil {
		return UnarchiveMigResult{}, fmt.Errorf("mod unarchive: http client required")
	}
	if c.BaseURL == nil {
		return UnarchiveMigResult{}, fmt.Errorf("mod unarchive: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return UnarchiveMigResult{}, fmt.Errorf("mod unarchive: mod ref is required")
	}

	// PATCH /v1/mods/{mod_ref}/unarchive
	endpoint := c.BaseURL.JoinPath("v1", "mods", c.MigRef.String(), "unarchive")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint.String(), nil)
	if err != nil {
		return UnarchiveMigResult{}, fmt.Errorf("mod unarchive: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return UnarchiveMigResult{}, fmt.Errorf("mod unarchive: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		var result UnarchiveMigResult
		if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return UnarchiveMigResult{}, fmt.Errorf("mod unarchive: decode response: %w", err)
		}
		return result, nil
	}

	return UnarchiveMigResult{}, decodeHTTPError(resp, "mod unarchive")
}

// SetModSpecCommand creates a new spec row and updates mods.spec_id.
// Endpoint: POST /v1/mods/{mod_ref}/specs
// Sets the mod's current spec by creating a new spec row.
type SetModSpecCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	MigRef    types.MigRef    // Required: mod ID or name.
	Spec      json.RawMessage // Required: spec content (YAML/JSON parsed to JSON).
	Name      *string         // Optional: spec name.
	CreatedBy *string         // Optional: creator identifier.
}

// SetModSpecResult contains the response from setting a mod spec.
type SetModSpecResult struct {
	ID        types.SpecID `json:"id"` // spec_id
	CreatedAt time.Time    `json:"created_at"`
}

// Run executes POST /v1/mods/{mod_ref}/specs to set the mod's spec.
func (c SetModSpecCommand) Run(ctx context.Context) (SetModSpecResult, error) {
	if c.Client == nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: http client required")
	}
	if c.BaseURL == nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: mod ref is required")
	}
	if len(c.Spec) == 0 {
		return SetModSpecResult{}, fmt.Errorf("mod spec set: spec is required")
	}

	// Build request payload with spec content, optional name, and created_by.
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

	// POST /v1/mods/{mod_ref}/specs
	endpoint := c.BaseURL.JoinPath("v1", "mods", c.MigRef.String(), "specs")
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
		if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return SetModSpecResult{}, fmt.Errorf("mod spec set: decode response: %w", err)
		}
		return result, nil
	}

	return SetModSpecResult{}, decodeHTTPError(resp, "mod spec set")
}

// ResolveModByNameCommand attempts to resolve a mod reference (ID or name) to a mod ID.
// It queries the server to find an exact name match, supporting both ID and name lookups.
// This command does NOT use any client-side heuristics to distinguish IDs from names;
// it always queries the server for resolution.
type ResolveModByNameCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  types.MigRef // Mod reference (could be ID or name).
}

// Run attempts to resolve a mod ID from a name reference.
// Returns the mod ID if found by exact name match, or the reference as-is if no match.
// No client-side heuristics are used to distinguish IDs from names.
func (c ResolveModByNameCommand) Run(ctx context.Context) (string, error) {
	if c.Client == nil {
		return "", fmt.Errorf("resolve mod: http client required")
	}
	if c.BaseURL == nil {
		return "", fmt.Errorf("resolve mod: base url required")
	}
	if err := c.MigRef.Validate(); err != nil {
		return "", fmt.Errorf("resolve mod: mod reference is required")
	}
	ref := c.MigRef.String()

	// Try to find by name using the list endpoint with name filter.
	// No heuristics - always query the server.
	listCmd := ListMigsCommand{
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
		return matches[0].ID.String(), nil
	default:
		// Multiple exact matches should not happen (name is unique), but handle gracefully.
		return "", fmt.Errorf("resolve mod: multiple mods found with name %q", ref)
	}
}
