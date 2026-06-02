package runs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

type RunSBOMResult struct {
	RunID        domaintypes.RunID
	View         string
	Packages     []migsapi.RunSBOMPackage
	DiffPackages []migsapi.RunSBOMDiffPackage
}

type GetRunSBOMCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	View    string
}

func (c GetRunSBOMCommand) Run(ctx context.Context) (RunSBOMResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return RunSBOMResult{}, fmt.Errorf("run sbom: %w", err)
	}
	view := strings.TrimSpace(c.View)
	if view != "pre" && view != "post" && view != "diff" {
		return RunSBOMResult{}, errors.New("run sbom: view must be pre, post, or diff")
	}
	if c.RunID.IsZero() {
		return RunSBOMResult{}, errors.New("run sbom: run id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "sbom", view)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return RunSBOMResult{}, fmt.Errorf("run sbom: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return RunSBOMResult{}, fmt.Errorf("run sbom: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		msg := httpx.ReadErrorMessage(resp.Body, resp.Status, httpx.MaxErrorBodyBytes)
		if resp.StatusCode == http.StatusBadRequest {
			return RunSBOMResult{}, errors.New(msg)
		}
		return RunSBOMResult{}, fmt.Errorf("run sbom: %s", msg)
	}

	if view == "diff" {
		var result migsapi.RunSBOMDiffResponse
		if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
			return RunSBOMResult{}, fmt.Errorf("run sbom: decode response: %w", err)
		}
		return RunSBOMResult{RunID: result.RunID, View: result.View, DiffPackages: result.Packages}, nil
	}

	var result migsapi.RunSBOMPackagesResponse
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return RunSBOMResult{}, fmt.Errorf("run sbom: decode response: %w", err)
	}
	return RunSBOMResult{RunID: result.RunID, View: result.View, Packages: result.Packages}, nil
}

func RenderRunSBOM(w io.Writer, result RunSBOMResult) error {
	if w == nil {
		w = io.Discard
	}
	if result.View == "diff" {
		return RenderSBOMDiff(w, result.DiffPackages)
	}
	_, err := fmt.Fprintln(w, formatSBOMPackageTable(result.Packages))
	return err
}

func RenderSBOMDiff(w io.Writer, packages []migsapi.RunSBOMDiffPackage) error {
	if w == nil {
		w = io.Discard
	}
	_, err := fmt.Fprintln(w, formatSBOMDiffBlock(packages))
	return err
}

func formatSBOMPackageTable(packages []migsapi.RunSBOMPackage) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%-24s %s\n", "Package", "Version")
	for _, pkg := range packages {
		_, _ = fmt.Fprintf(&b, "%-24s %s\n", pkg.Package, pkg.Version)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatSBOMDiffBlock(packages []migsapi.RunSBOMDiffPackage) string {
	var b strings.Builder
	b.WriteString("SBOM diff\n")
	packageWidth := maxSBOMDiffPackageWidth(packages)
	for _, pkg := range packages {
		pre := strings.TrimSpace(pkg.VersionPre)
		if pre == "" {
			pre = "-"
		}
		post := strings.TrimSpace(pkg.VersionPost)
		if post == "" {
			post = "-"
		}
		_, _ = fmt.Fprintf(&b, "%-*s %-16s -> %s\n", packageWidth, pkg.Package, pre, post)
	}
	return strings.TrimRight(b.String(), "\n")
}

func maxSBOMDiffPackageWidth(packages []migsapi.RunSBOMDiffPackage) int {
	width := 0
	for _, pkg := range packages {
		if n := len(pkg.Package); n > width {
			width = n
		}
	}
	return width
}
