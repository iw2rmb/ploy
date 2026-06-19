package spec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestHandleSpecSchemaPrintsEmbeddedSchema(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Handle([]string{"schema"}, &stdout, &stderr); err != nil {
		t.Fatalf("Handle(schema) error = %v", err)
	}
	want, err := contracts.MigSpecSchemaJSON()
	if err != nil {
		t.Fatalf("MigSpecSchemaJSON() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != strings.TrimSpace(string(want)) {
		t.Fatalf("schema output does not match embedded schema")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestHandleSpecValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    string
		files   map[string]string
		wantErr string
	}{
		{
			name: "valid",
			spec: "steps:\n  - image: docker.io/test/mig:latest\n",
		},
		{
			name:    "unknown root key",
			spec:    "version: old\nsteps:\n  - image: docker.io/test/mig:latest\n",
			wantErr: "validate spec",
		},
		{
			name:    "unknown nested build gate key",
			spec:    "steps:\n  - image: docker.io/test/mig:latest\nbuild_gate:\n  enabled: true\n",
			wantErr: "additional properties 'enabled' not allowed",
		},
		{
			name:    "missing hydra input file",
			spec:    "steps:\n  - image: docker.io/test/mig:latest\n    in:\n      - ./missing.yaml:missing.yaml\n",
			wantErr: "validate local file records",
		},
		{
			name: "amata include not mounted",
			spec: "steps:\n  - image: docker.io/test/mig:latest\n    in:\n      - ./amata.yaml:amata.yaml\n",
			files: map[string]string{
				"amata.yaml":           "flows:\n  audit: !include ./gradle-assemble.yaml#/flows/audit\n",
				"gradle-assemble.yaml": "flows:\n  audit:\n    steps: []\n",
			},
			wantErr: "target /in/gradle-assemble.yaml is not mounted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "spec.yaml")
			if err := os.WriteFile(path, []byte(tt.spec), 0o644); err != nil {
				t.Fatalf("write spec: %v", err)
			}
			for rel, content := range tt.files {
				if err := os.WriteFile(filepath.Join(filepath.Dir(path), rel), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := Handle([]string{"validate", path}, &stdout, &stderr)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Handle(validate) error = %v", err)
				}
				if !strings.Contains(stderr.String(), "Validated spec "+path) {
					t.Fatalf("stderr = %q, want validation message", stderr.String())
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Handle(validate) error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestHandleSpecPush(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		files          map[string]string
		afterCommit    func(t *testing.T, repo string)
		responseByName map[string]bool
		wantErr        string
		wantOut        []string
		wantPosts      int
		assertRequests func(t *testing.T, requests []domainapi.PublishNamedSpecRequest)
	}{
		{
			name:   "clean repo publishes matching yaml",
			origin: "git@github.com:acme/service.git",
			files: map[string]string{
				"mig.yaml": namedSpecYAML("upgrade-java", ""),
			},
			wantOut:   []string{"STATE", "updated", "upgrade-java", "github.com/acme/service"},
			wantPosts: 1,
			assertRequests: func(t *testing.T, requests []domainapi.PublishNamedSpecRequest) {
				req := requests[0]
				if req.Name != "upgrade-java" {
					t.Fatalf("request name = %q", req.Name)
				}
				if req.Source.Domain != "github.com" || req.Source.Repo != "acme/service" {
					t.Fatalf("source = %+v", req.Source)
				}
				if req.SHA == "" || req.SourceCommittedAt.IsZero() {
					t.Fatalf("missing git provenance: sha=%q committed_at=%s", req.SHA, req.SourceCommittedAt)
				}
			},
		},
		{
			name:   "dirty tracked file fails before publish",
			origin: "https://github.com/acme/service.git",
			files: map[string]string{
				"mig.yaml": namedSpecYAML("upgrade-java", ""),
			},
			afterCommit: func(t *testing.T, repo string) {
				t.Helper()
				writeTestFile(t, filepath.Join(repo, "mig.yaml"), namedSpecYAML("upgrade-java", "CHANGED: true\n"))
			},
			wantErr:   "git worktree must be clean",
			wantPosts: 0,
		},
		{
			name:   "untracked file fails before publish",
			origin: "https://github.com/acme/service.git",
			files: map[string]string{
				"mig.yaml": namedSpecYAML("upgrade-java", ""),
			},
			afterCommit: func(t *testing.T, repo string) {
				t.Helper()
				writeTestFile(t, filepath.Join(repo, "untracked.txt"), "untracked\n")
			},
			wantErr:   "git worktree must be clean",
			wantPosts: 0,
		},
		{
			name:   "non matching yaml and yml are ignored",
			origin: "https://github.com/acme/service.git",
			files: map[string]string{
				"not-a-spec.yaml": "name: missing-version\n",
				"ignored.yml":     namedSpecYAML("ignored", ""),
			},
			wantOut:   []string{"No named specs found"},
			wantPosts: 0,
		},
		{
			name:   "ignored yaml is excluded from discovery",
			origin: "https://github.com/acme/service.git",
			files: map[string]string{
				".gitignore":   "ignored.yaml\n",
				"ignored.yaml": namedSpecYAML("ignored", ""),
			},
			wantOut:   []string{"No named specs found"},
			wantPosts: 0,
		},
		{
			name:   "skipped response renders skipped",
			origin: "https://github.com/acme/service.git",
			files: map[string]string{
				"mig.yaml": namedSpecYAML("upgrade-java", ""),
			},
			responseByName: map[string]bool{"upgrade-java": true},
			wantOut:        []string{"skipped", "upgrade-java"},
			wantPosts:      1,
		},
		{
			name:   "authoring input is compiled into bundle map",
			origin: "https://github.com/acme/service.git",
			files: map[string]string{
				"config.txt": "payload\n",
				"mig.yaml": namedSpecYAML("upgrade-java", `    in:
      - ./config.txt:config.txt
`),
			},
			wantOut:   []string{"updated", "upgrade-java"},
			wantPosts: 1,
			assertRequests: func(t *testing.T, requests []domainapi.PublishNamedSpecRequest) {
				var spec map[string]any
				if err := json.Unmarshal(requests[0].Spec, &spec); err != nil {
					t.Fatalf("unmarshal published spec: %v", err)
				}
				bundleMap, ok := spec["bundle_map"].(map[string]any)
				if !ok || len(bundleMap) != 1 {
					t.Fatalf("bundle_map = %#v, want one compiled bundle", spec["bundle_map"])
				}
				steps := spec["steps"].([]any)
				step := steps[0].(map[string]any)
				in := step["in"].([]any)
				entry := in[0].(string)
				if !strings.Contains(entry, ":/in/config.txt") {
					t.Fatalf("steps[0].in[0] = %q, want compiled /in/config.txt entry", entry)
				}
			},
		},
		{
			name:   "invalid source origin fails clearly",
			origin: "file:///tmp/service.git",
			files: map[string]string{
				"mig.yaml": namedSpecYAML("upgrade-java", ""),
			},
			wantErr:   "origin remote must normalize to domain/namespace/repo",
			wantPosts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSpecPushRepo(t, tt.origin, tt.files)
			if tt.afterCommit != nil {
				tt.afterCommit(t, repo)
			}

			server := newNamedSpecTestServer(t, tt.responseByName)
			t.Setenv("PLOY_SERVER_URL", server.url)
			t.Setenv("PLOY_AUTH_TOKEN", "test-token")
			t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
			t.Setenv("USER", "spec-tester")

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := Handle([]string{"push", repo}, &stdout, &stderr)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Handle(push) error = %v, want containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Handle(push) error = %v", err)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want containing %q", stdout.String(), want)
				}
			}
			requests := server.requests()
			if len(requests) != tt.wantPosts {
				t.Fatalf("publish posts = %d, want %d", len(requests), tt.wantPosts)
			}
			assertRenderedRequestSHAs(t, stdout.String(), requests)
			if tt.assertRequests != nil {
				tt.assertRequests(t, requests)
			}
		})
	}
}

func TestHandleSpecList(t *testing.T) {
	server := newNamedSpecTestServer(t, nil)
	t.Setenv("PLOY_SERVER_URL", server.url)
	t.Setenv("PLOY_AUTH_TOKEN", "test-token")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Handle([]string{"ls"}, &stdout, &stderr); err != nil {
		t.Fatalf("Handle(ls) error = %v", err)
	}
	for _, want := range []string{"NAME", "SOURCE", "upgrade-java", "github.com/acme/service", "01234567"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want containing %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), "012345678") {
		t.Fatalf("stdout = %q, want 8-character SHA rendering", stdout.String())
	}
}

func TestProbeNamedSpecFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		wantName string
		wantOK   bool
	}{
		{name: "named spec with nested custom tag", content: "apiVersion: ploy.mig/v1alpha1\nname: upgrade-java\nsteps:\n  - image: test\n    command: !include ./cmd.yaml\n", wantName: "upgrade-java", wantOK: true},
		{name: "missing api version", content: "name: upgrade-java\n", wantOK: false},
		{name: "yml extension is not part of probe", content: "apiVersion: ploy.mig/v1alpha1\nname: upgrade-java\n", wantName: "upgrade-java", wantOK: true},
		{name: "invalid yaml", content: "apiVersion: [\n", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "spec.yaml")
			writeTestFile(t, path, tt.content)
			probe, ok := probeNamedSpecFile(path)
			if ok != tt.wantOK {
				t.Fatalf("probeNamedSpecFile() ok = %v, want %v", ok, tt.wantOK)
			}
			if probe.Name != tt.wantName {
				t.Fatalf("probe name = %q, want %q", probe.Name, tt.wantName)
			}
		})
	}
}

func TestParseNamedSpecSourceOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		origin     string
		wantDomain string
		wantRepo   string
		wantErr    string
	}{
		{name: "https", origin: "https://github.com/acme/service.git", wantDomain: "github.com", wantRepo: "acme/service"},
		{name: "ssh scp", origin: "git@gitlab.com:team/service.git", wantDomain: "gitlab.com", wantRepo: "team/service"},
		{name: "path-like file origin", origin: "file:///tmp/service.git", wantErr: "domain/namespace/repo"},
		{name: "missing namespace", origin: "https://github.com/service.git", wantErr: "domain/namespace/repo"},
		{name: "empty part", origin: "https://github.com/acme//service.git", wantErr: "empty path component"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			domain, repo, err := parseNamedSpecSourceOrigin(tt.origin)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseNamedSpecSourceOrigin() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseNamedSpecSourceOrigin() error = %v", err)
			}
			if domain != tt.wantDomain || repo != tt.wantRepo {
				t.Fatalf("source = %s/%s, want %s/%s", domain, repo, tt.wantDomain, tt.wantRepo)
			}
		})
	}
}

type namedSpecTestServer struct {
	url      string
	mu       sync.Mutex
	captured []domainapi.PublishNamedSpecRequest
	skipped  map[string]bool
}

func newNamedSpecTestServer(t *testing.T, skipped map[string]bool) *namedSpecTestServer {
	t.Helper()
	s := &namedSpecTestServer{skipped: skipped}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/spec-bundles" && r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/v1/spec-bundles" && r.Method == http.MethodPost:
			data, _ := io.ReadAll(r.Body)
			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bundle_id": "bundle-" + hash[:12],
				"cid":       "bafy" + hash[:32],
				"digest":    "sha256:" + hash,
			})
		case r.URL.Path == "/v1/specs" && r.Method == http.MethodPost:
			var req domainapi.PublishNamedSpecRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			s.captured = append(s.captured, req)
			s.mu.Unlock()
			skipped := s.skipped[req.Name]
			if skipped {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusCreated)
			}
			_ = json.NewEncoder(w).Encode(domainapi.NamedSpecSummary{
				ID:                "spec001",
				Name:              req.Name,
				Description:       req.Description,
				Source:            req.Source,
				SHA:               req.SHA,
				SourceCommittedAt: req.SourceCommittedAt,
				CreatedAt:         req.SourceCommittedAt.Add(time.Minute),
				Skipped:           skipped,
			})
		case r.URL.Path == "/v1/specs" && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("named"); got != "true" {
				http.Error(w, "missing named=true", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(domainapi.NamedSpecListResponse{Specs: []domainapi.NamedSpecSummary{{
				ID:                "spec001",
				Name:              "upgrade-java",
				Source:            domainapi.NamedSpecSource{Domain: "github.com", Repo: "acme/service"},
				SHA:               "0123456789abcdef0123456789abcdef01234567",
				SourceCommittedAt: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
				CreatedAt:         time.Date(2026, 6, 19, 12, 1, 0, 0, time.UTC),
			}}})
		default:
			http.Error(w, fmt.Sprintf("unexpected %s %s", r.Method, r.URL.String()), http.StatusInternalServerError)
		}
	}))
	s.url = srv.URL
	t.Cleanup(srv.Close)
	return s
}

func (s *namedSpecTestServer) requests() []domainapi.PublishNamedSpecRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domainapi.PublishNamedSpecRequest(nil), s.captured...)
}

func assertRenderedRequestSHAs(t *testing.T, stdout string, requests []domainapi.PublishNamedSpecRequest) {
	t.Helper()
	for _, req := range requests {
		if len(req.SHA) <= 8 {
			continue
		}
		if !strings.Contains(stdout, req.SHA[:8]) {
			t.Fatalf("stdout = %q, want rendered SHA %q", stdout, req.SHA[:8])
		}
		if strings.Contains(stdout, req.SHA[:9]) {
			t.Fatalf("stdout = %q, want 8-character SHA rendering", stdout)
		}
	}
}

func initSpecPushRepo(t *testing.T, origin string, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "spec@example.test")
	runGit(t, repo, "config", "user.name", "Spec Tester")
	for rel, content := range files {
		writeTestFile(t, filepath.Join(repo, rel), content)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	runGit(t, repo, "remote", "add", "origin", origin)
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func namedSpecYAML(name string, stepExtra string) string {
	return `apiVersion: ploy.mig/v1alpha1
name: ` + name + `
description: Test spec
steps:
  - image: docker.io/test/mig:latest
` + stepExtra
}
