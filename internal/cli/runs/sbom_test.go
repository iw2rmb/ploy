package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestGetRunSBOMCommandDiffAndRender(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(migsapi.RunSBOMDiffResponse{
			RunID: runID,
			View:  "diff",
			Packages: []migsapi.RunSBOMDiffPackage{
				{Package: "alpha", VersionPre: "1.0", VersionPost: "2.0", Change: "changed"},
				{Package: "beta", VersionPost: "1.0", Change: "added"},
				{Package: "gamma", VersionPre: "3.0", Change: "removed"},
			},
		})
	}))
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	result, err := GetRunSBOMCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		RunID:   runID,
		View:    "diff",
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("GetRunSBOMCommand.Run error: %v", err)
	}
	if gotPath != "/v1/runs/"+runID.String()+"/sbom/diff" {
		t.Fatalf("path=%q, want diff endpoint", gotPath)
	}

	var out bytes.Buffer
	if err := RenderRunSBOM(&out, result); err != nil {
		t.Fatalf("RenderRunSBOM error: %v", err)
	}
	want := strings.Join([]string{
		"SBOM diff",
		"alpha                    1.0              -> 2.0",
		"beta                     -                -> 1.0",
		"gamma                    3.0              -> -",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("output=%q, want %q", out.String(), want)
	}
}

func TestGetRunSBOMCommandBadRequestUsesControlPlaneText(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "build gate disabled for run", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	_, err = GetRunSBOMCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		RunID:   runID,
		View:    "diff",
	}.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "build gate disabled for run" {
		t.Fatalf("error=%q, want control-plane body", err.Error())
	}
}

func TestRenderRunSBOMPackageTable(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := RenderRunSBOM(&out, RunSBOMResult{
		View: "pre",
		Packages: []migsapi.RunSBOMPackage{
			{Package: "alpha", Version: "1.0.0"},
			{Package: "beta", Version: "2.0.0"},
		},
	})
	if err != nil {
		t.Fatalf("RenderRunSBOM error: %v", err)
	}
	want := strings.Join([]string{
		"Package                  Version",
		"alpha                    1.0.0",
		"beta                     2.0.0",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("output=%q, want %q", out.String(), want)
	}
}
