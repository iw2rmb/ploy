package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestRunControllerPersistGateSBOM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fileBody   string
		wantUpload bool
		wantRows   []migsapi.RunSBOMPackage
	}{
		{
			name:       "present sbom uploads parsed rows",
			wantUpload: true,
			fileBody: `{
  "spdxVersion":"SPDX-2.3",
  "packages":[
    {"name":"Org.Example:Lib-A","versionInfo":"1.0.0"},
    {"name":"org.example:lib-a","versionInfo":"1.0.0"},
    {"name":"org.example:lib-b","versionInfo":"2.0.0"}
  ]
}`,
			wantRows: []migsapi.RunSBOMPackage{
				{Package: "org.example:lib-a", Version: "1.0.0"},
				{Package: "org.example:lib-b", Version: "2.0.0"},
			},
		},
		{
			name: "missing sbom skips upload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			shareDir := t.TempDir()
			if tt.fileBody != "" {
				if err := os.WriteFile(filepath.Join(shareDir, gateSBOMFilename), []byte(tt.fileBody), 0o600); err != nil {
					t.Fatalf("write sbom: %v", err)
				}
			}

			var gotPath string
			var gotPackages []migsapi.RunSBOMPackage
			uploads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				uploads++
				gotPath = r.URL.Path
				if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Fatalf("Authorization = %q, want Bearer test-token", got)
				}
				if got := r.Header.Get("PLOY_NODE_UUID"); got != testNodeID {
					t.Fatalf("PLOY_NODE_UUID = %q, want %s", got, testNodeID)
				}
				var payload migsapi.JobSBOMUploadRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				gotPackages = payload.Packages
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			controller := newTestController(t, newAgentConfig(server.URL))
			req := StartRunRequest{
				RunID:   types.NewRunID(),
				JobID:   types.JobID("job-sbom-test"),
				JobType: types.JobTypePreGate,
			}

			if err := controller.persistGateSBOM(context.Background(), req, shareDir); err != nil {
				t.Fatalf("persistGateSBOM error: %v", err)
			}

			if tt.wantUpload {
				if uploads != 1 {
					t.Fatalf("uploads = %d, want 1", uploads)
				}
				if gotPath != "/v1/jobs/job-sbom-test/sbom" {
					t.Fatalf("path = %q, want /v1/jobs/job-sbom-test/sbom", gotPath)
				}
				if len(gotPackages) != len(tt.wantRows) {
					t.Fatalf("packages len = %d, want %d: %+v", len(gotPackages), len(tt.wantRows), gotPackages)
				}
				for i, want := range tt.wantRows {
					if gotPackages[i] != want {
						t.Fatalf("packages[%d] = %+v, want %+v", i, gotPackages[i], want)
					}
				}
				return
			}
			if uploads != 0 {
				t.Fatalf("uploads = %d, want 0", uploads)
			}
		})
	}
}
