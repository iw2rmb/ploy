package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestRegisterRoutesMatchesOpenAPI verifies that all endpoints documented in
// docs/api/OpenAPI.yaml are actually mounted by RegisterRoutes. It probes each
// method+path combination and asserts the server does not return 404 Not Found
// (i.e., the route exists). Authorization and payload validation may still
// cause 4xx responses, which are acceptable for this coverage check.
func TestRegisterRoutesMatchesOpenAPI(t *testing.T) {
	// Load OpenAPI spec
	specPath := filepath.Join("..", "..", "..", "docs", "api", "OpenAPI.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read OpenAPI.yaml: %v", err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse OpenAPI.yaml: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths not found in OpenAPI.yaml")
	}

	// Prepare a test server instance with insecure authorizer so requests
	// do not require mTLS. We'll exercise three roles to cover all routes.
	newServer := func(defaultRole auth.Role) (*server.HTTPServer, *server.EventsService) {
		authz := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: defaultRole})
		srv, err := server.NewHTTPServer(server.HTTPOptions{Authorizer: authz})
		if err != nil {
			t.Fatalf("http server: %v", err)
		}
		ev, err := server.NewEventsService(server.EventsOptions{})
		if err != nil {
			t.Fatalf("events: %v", err)
		}
		st := &mockStore{} // minimal store; handlers may still return 4xx
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)
		cfg := NewConfigHolder(config.GitLabConfig{}, nil)
		RegisterRoutes(srv, st, bs, bp, ev, cfg, "test-secret")
		return srv, ev
	}

	// Spin three servers for the three role classes.
	srvCP, _ := newServer(auth.RoleControlPlane)
	srvWorker, _ := newServer(auth.RoleWorker)
	srvAdmin, _ := newServer(auth.RoleCLIAdmin)

	// Helper to probe an endpoint against one server.
	probe := func(srv *server.HTTPServer, method, path string) int {
		// Replace templated vars with sample values.
		sample := path
		sample = strings.ReplaceAll(sample, "{id}", uuid.New().String())
		sample = strings.ReplaceAll(sample, "{stage}", uuid.New().String())

		var body io.Reader
		if method == http.MethodPost || method == http.MethodDelete || method == http.MethodPut || method == http.MethodPatch {
			// Minimal JSON body to pass decoder; specific handlers may still 400.
			body = bytes.NewBufferString("{}")
		}
		// Short timeout to avoid blocking SSE streams.
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		req := httptest.NewRequest(method, sample, body).WithContext(ctx)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rr := httptest.NewRecorder()
		handler := srv.Handler()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	// allowedMissing contains paths that are documented in OpenAPI but not mounted
	// in RegisterRoutes (typically PKI endpoints that are mounted separately).
	// HTTP Build Gate endpoints have been removed from both OpenAPI and code;
	// gate execution now runs as part of unified jobs queue.
	allowedMissing := map[string]struct{}{
		"/v1/pki/sign":        {},
		"/v1/pki/sign/client": {},
		"/v1/pki/sign/admin":  {},
	}

	for p, item := range paths {
		// Resolve $ref if present; otherwise use the inline methods map.
		var methods map[string]any
		if m, ok := item.(map[string]any); ok {
			if ref, ok := m["$ref"].(string); ok {
				refPath := filepath.Join("..", "..", "..", "docs", "api", ref)
				refData, err := os.ReadFile(refPath)
				if err != nil {
					t.Fatalf("read %s: %v", refPath, err)
				}
				if err := yaml.Unmarshal(refData, &methods); err != nil {
					t.Fatalf("parse %s: %v", refPath, err)
				}
			} else {
				methods = m
			}
		} else {
			t.Fatalf("invalid path item for %s", p)
		}

		for method := range methods {
			lm := strings.ToLower(method)
			if lm != "get" && lm != "post" && lm != "delete" && lm != "put" && lm != "patch" {
				continue
			}
			if _, skip := allowedMissing[p]; skip {
				continue
			}
			t.Run(strings.ToUpper(lm)+" "+p, func(t *testing.T) {
				codes := []int{
					probe(srvCP, strings.ToUpper(lm), p),
					probe(srvWorker, strings.ToUpper(lm), p),
					probe(srvAdmin, strings.ToUpper(lm), p),
				}
				// Consider any status other than 404 as evidence the route is mounted.
				ok := false
				for _, code := range codes {
					if code != http.StatusNotFound {
						ok = true
						break
					}
				}
				if !ok {
					// Provide debugging info: show codes per role.
					payload, _ := json.Marshal(codes)
					t.Fatalf("route not mounted for any role; got codes %s", string(payload))
				}
			})
		}
	}
}
