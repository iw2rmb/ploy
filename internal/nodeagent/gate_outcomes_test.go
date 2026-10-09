package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestGateOutcomesPublishJobLocalJSONAndKeepPartialResults(t *testing.T) {
	for _, tc := range []struct {
		name, cves  string
		wantError   bool
		wantUploads int
	}{
		{"pair", `{"matches":[]}`, false, 2},
		{"missing CVEs", "", false, 1},
		{"invalid CVEs", `{`, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, share := t.TempDir(), t.TempDir()
			sbom := `{"spdxVersion":"SPDX-2.3","packages":[]}`
			if err := os.WriteFile(filepath.Join(out, gateSBOMFilename), []byte(sbom), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(share, gateSBOMFilename), []byte(`{"packages":[{"name":"stale","versionInfo":"1"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.cves != "" {
				if err := os.WriteFile(filepath.Join(out, "grype.json"), []byte(tc.cves), 0600); err != nil {
					t.Fatal(err)
				}
			}
			uploads := map[string]string{}
			rowsPosted := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/jobs/gate-job/sbom" {
					rowsPosted = true
					var payload struct {
						Packages []any `json:"packages"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if len(payload.Packages) != 0 {
						t.Error("used stale shared SBOM")
					}
					w.WriteHeader(http.StatusOK)
					return
				}
				if r.URL.Path != "/v1/runs/gate-run/jobs/gate-job/artifact" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				var payload struct {
					Name   string `json:"name"`
					Bundle []byte `json:"bundle"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					return
				}
				zr, err := gzip.NewReader(bytes.NewReader(payload.Bundle))
				if err != nil {
					t.Error(err)
					return
				}
				defer zr.Close()
				tr := tar.NewReader(zr)
				header, err := tr.Next()
				if err != nil {
					t.Error(err)
					return
				}
				expected := "grype.json"
				if payload.Name == "sbom" {
					expected = gateSBOMFilename
				}
				if header.Name != expected {
					t.Errorf("entry=%s want %s", header.Name, expected)
				}
				body, err := io.ReadAll(tr)
				if err != nil {
					t.Error(err)
					return
				}
				uploads[payload.Name] = string(body)
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]string{"artifact_bundle_id": "fixture", "cid": "fixture"})
			}))
			defer server.Close()
			controller := newTestController(t, newAgentConfig(server.URL))
			err := controller.persistGateOutcomes(context.Background(), StartRunRequest{RunID: types.RunID("gate-run"), JobID: types.JobID("gate-job"), JobType: types.JobTypePreGate}, step.JobMounts{Out: out, Share: share})
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			if len(uploads) != tc.wantUploads || uploads["sbom"] != sbom || !rowsPosted {
				t.Fatalf("uploads=%v rows=%v", uploads, rowsPosted)
			}
			if tc.wantUploads == 2 && uploads["cves"] != tc.cves {
				t.Fatal("changed CVE JSON")
			}
			if err := clearGateOutcomes(out); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(out, gateSBOMFilename)); !os.IsNotExist(err) {
				t.Fatal("old SBOM survived retry cleanup")
			}
		})
	}
}
