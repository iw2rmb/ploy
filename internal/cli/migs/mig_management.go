// Package migs provides CLI client implementations for Migs operations.
// This file implements mig project management commands (add, list, remove, archive, unarchive).
//
// These commands call the server endpoints:
// - POST /v1/migs (create mig)
// - GET /v1/migs (list migs)
// - DELETE /v1/migs/{mig_ref} (delete mig)
// - PATCH /v1/migs/{mig_ref}/archive (archive mig)
// - PATCH /v1/migs/{mig_ref}/unarchive (unarchive mig)
//
// These commands implement the mig management surfaces (create, list, delete, archive).
package migs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sharedclient "github.com/iw2rmb/ploy/internal/client"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// AddMigCommand creates a new mig project.
// Endpoint: POST /v1/migs
// Creates a mig with unique name and optional initial spec.
type AddMigCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Name      string           // Required: unique mig name.
	Spec      *json.RawMessage // Optional: initial spec (creates spec row and sets migs.spec_id).
	CreatedBy *string          // Optional: creator identifier.
}

// AddMigResult contains the response from creating a mig.
type AddMigResult = domainapi.MigSummary

// Run executes POST /v1/migs to create a mig project.
func (c AddMigCommand) Run(ctx context.Context) (AddMigResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return AddMigResult{}, fmt.Errorf("mig add: %w", err)
	}
	if strings.TrimSpace(c.Name) == "" {
		return AddMigResult{}, fmt.Errorf("mig add: name is required")
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

	// POST /v1/migs to create the mig.
	endpoint := c.BaseURL.JoinPath("v1", "migs")
	return httpx.DoJSON[AddMigResult](ctx, c.Client, http.MethodPost, endpoint.String(), req, http.StatusCreated, "mig add")
}

// RemoveMigCommand deletes a mig project.
// Endpoint: DELETE /v1/migs/{mig_ref}
// Refuses deletion if the mig has any runs.
type RemoveMigCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  types.MigRef // Required: mig ID or name to delete.
}

// Run executes DELETE /v1/migs/{mig_ref} to delete a mig.
func (c RemoveMigCommand) Run(ctx context.Context) error {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return fmt.Errorf("mig remove: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return fmt.Errorf("mig remove: mig ref is required")
	}

	// DELETE /v1/migs/{mig_ref}
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("mig remove: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mig remove: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	// 204 No Content indicates success.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return httpx.WrapError("mig remove", resp.Status, resp.Body)
}

// SetMigArchivedCommand sets the archived state of a mig project.
type SetMigArchivedCommand struct {
	Client   *http.Client
	BaseURL  *url.URL
	MigRef   types.MigRef // Required: mig ID or name.
	Archived bool
}

// Run executes the archive or unarchive endpoint selected by Archived.
func (c SetMigArchivedCommand) Run(ctx context.Context) (domainapi.MigArchiveResponse, error) {
	action := "unarchive"
	if c.Archived {
		action = "archive"
	}
	op := "mig " + action
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.MigArchiveResponse{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return domainapi.MigArchiveResponse{}, fmt.Errorf("%s: mig ref is required", op)
	}

	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), action)
	return httpx.DoJSON[domainapi.MigArchiveResponse](ctx, c.Client, http.MethodPatch, endpoint.String(), nil, http.StatusOK, op)
}

// SetMigSpecCommand creates a new spec row and updates migs.spec_id.
// Endpoint: POST /v1/migs/{mig_ref}/specs
// Sets the mig's current spec by creating a new spec row.
type SetMigSpecCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	MigRef    types.MigRef    // Required: mig ID or name.
	Spec      json.RawMessage // Required: spec content (YAML/JSON parsed to JSON).
	Name      *string         // Optional: spec name.
	CreatedBy *string         // Optional: creator identifier.
}

// SetMigSpecResult contains the response from setting a mig spec.
type SetMigSpecResult struct {
	ID        types.SpecID `json:"id"` // spec_id
	CreatedAt time.Time    `json:"created_at"`
}

// Run executes POST /v1/migs/{mig_ref}/specs to set the mig's spec.
func (c SetMigSpecCommand) Run(ctx context.Context) (SetMigSpecResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return SetMigSpecResult{}, fmt.Errorf("mig spec set: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return SetMigSpecResult{}, fmt.Errorf("mig spec set: mig ref is required")
	}
	if len(c.Spec) == 0 {
		return SetMigSpecResult{}, fmt.Errorf("mig spec set: spec is required")
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

	// POST /v1/migs/{mig_ref}/specs
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "specs")
	return httpx.DoJSON[SetMigSpecResult](ctx, c.Client, http.MethodPost, endpoint.String(), req, http.StatusCreated, "mig spec set")
}

// ResolveMigByNameCommand attempts to resolve a mig reference (ID or name) to a mig ID.
// It queries the server to find an exact name match, supporting both ID and name lookups.
// This command does NOT use any client-side heuristics to distinguish IDs from names;
// it always queries the server for resolution.
type ResolveMigByNameCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  types.MigRef // Mig reference (could be ID or name).
}

// Run attempts to resolve a mig ID from a name reference.
// Returns the mig ID if found by exact name match, or the reference as-is if no match.
// No client-side heuristics are used to distinguish IDs from names.
func (c ResolveMigByNameCommand) Run(ctx context.Context) (string, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return "", fmt.Errorf("resolve mig: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return "", fmt.Errorf("resolve mig: mig reference is required")
	}
	ref := c.MigRef.String()

	// Try to find by name using the list endpoint with name filter.
	// No heuristics - always query the server.
	listCmd := sharedclient.ListMigsCommand{
		Client:        c.Client,
		BaseURL:       c.BaseURL,
		Limit:         100,
		NameSubstring: &ref,
	}

	migs, err := listCmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve mig: %w", err)
	}

	// Find exact name match.
	var matches []domainapi.MigSummary
	for _, mig := range migs {
		if mig.Name == ref {
			matches = append(matches, mig)
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
		return "", fmt.Errorf("resolve mig: multiple migs found with name %q", ref)
	}
}
