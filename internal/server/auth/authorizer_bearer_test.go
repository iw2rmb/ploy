package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Mock querier
// ---------------------------------------------------------------------------

type mockQuerier struct {
	*store.Queries

	mu sync.Mutex

	checkAPITokenRevokedFunc         func(ctx context.Context, tokenID string) (pgtype.Timestamptz, error)
	checkBootstrapTokenRevokedFunc   func(ctx context.Context, tokenID string) (pgtype.Timestamptz, error)
	updateAPITokenLastUsedFunc       func(ctx context.Context, tokenID string) error
	updateBootstrapTokenLastUsedFunc func(ctx context.Context, tokenID string) error

	updateAPITokenLastUsedCalled       bool
	updateBootstrapTokenLastUsedCalled bool
}

func (m *mockQuerier) CheckAPITokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	if m.checkAPITokenRevokedFunc != nil {
		return m.checkAPITokenRevokedFunc(ctx, tokenID)
	}
	return pgtype.Timestamptz{}, pgx.ErrNoRows
}

func (m *mockQuerier) CheckBootstrapTokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	if m.checkBootstrapTokenRevokedFunc != nil {
		return m.checkBootstrapTokenRevokedFunc(ctx, tokenID)
	}
	return pgtype.Timestamptz{}, pgx.ErrNoRows
}

func (m *mockQuerier) UpdateAPITokenLastUsed(ctx context.Context, tokenID string) error {
	m.mu.Lock()
	m.updateAPITokenLastUsedCalled = true
	m.mu.Unlock()
	if m.updateAPITokenLastUsedFunc != nil {
		return m.updateAPITokenLastUsedFunc(ctx, tokenID)
	}
	return nil
}

func (m *mockQuerier) UpdateBootstrapTokenLastUsed(ctx context.Context, tokenID string) error {
	m.mu.Lock()
	m.updateBootstrapTokenLastUsedCalled = true
	m.mu.Unlock()
	if m.updateBootstrapTokenLastUsedFunc != nil {
		return m.updateBootstrapTokenLastUsedFunc(ctx, tokenID)
	}
	return nil
}

func (m *mockQuerier) APITokenLastUsedCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateAPITokenLastUsedCalled
}

func (m *mockQuerier) BootstrapTokenLastUsedCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateBootstrapTokenLastUsedCalled
}

// ---------------------------------------------------------------------------
// Shared constants and helpers
// ---------------------------------------------------------------------------

const testClusterID = "test-cluster"

func mustGenerateAPIToken(t *testing.T, role string, expiresAt time.Time) string {
	t.Helper()
	tok, err := GenerateAPIToken(testSecret, testClusterID, role, expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	return tok
}

// bearerTestHarness bundles common setup for bearer-token middleware tests.
type bearerTestHarness struct {
	t     *testing.T
	auth  *Authorizer
	mockQ *mockQuerier
}

func newBearerHarness(t *testing.T, opts Options) *bearerTestHarness {
	t.Helper()
	mq, _ := opts.Querier.(*mockQuerier)
	return &bearerTestHarness{t: t, auth: NewAuthorizer(opts), mockQ: mq}
}

func (h *bearerTestHarness) do(
	method, target, authHeader string,
	allowedRoles []Role,
	innerFn func(w http.ResponseWriter, r *http.Request),
) (rr *httptest.ResponseRecorder, called bool) {
	h.t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr = httptest.NewRecorder()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if innerFn != nil {
			innerFn(w, r)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})
	h.auth.Middleware(allowedRoles...)(inner).ServeHTTP(rr, req)
	return rr, called
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAuthorizerBearerToken_Rejected(t *testing.T) {
	validToken := mustGenerateAPIToken(t, string(RoleControlPlane), time.Now().Add(24*time.Hour))
	expiredToken := mustGenerateAPIToken(t, string(RoleControlPlane), time.Now().Add(-1*time.Hour))

	wrongSecretToken := func() string {
		tok, err := GenerateAPIToken("secret-two-12345678901234567890", testClusterID, string(RoleControlPlane), time.Now().Add(24*time.Hour))
		if err != nil {
			t.Fatalf("GenerateAPIToken: %v", err)
		}
		return tok
	}()

	tests := []struct {
		name       string
		authHeader string
		querier    *mockQuerier
		wantBody   string
	}{
		{"expired token", "Bearer " + expiredToken, nil, ""},
		{"revoked token", "Bearer " + validToken, &mockQuerier{
			checkAPITokenRevokedFunc: func(context.Context, string) (pgtype.Timestamptz, error) {
				return pgtype.Timestamptz{Time: time.Now(), Valid: true}, nil
			},
		}, "authentication failed"},
		{"missing Authorization header", "", nil, ""},
		{"malformed JWT", "Bearer invalid-token", nil, ""},
		{"missing Bearer prefix", "invalid-token", nil, ""},
		{"empty Bearer value", "Bearer ", nil, ""},
		{"Basic auth scheme", "Basic base64encodedcreds", nil, ""},
		{"wrong signing secret", "Bearer " + wrongSecretToken, nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.querier
			if q == nil {
				q = &mockQuerier{}
			}
			h := newBearerHarness(t, Options{
				TokenSecret: testSecret,
				Querier:     q,
			})

			rr, called := h.do(http.MethodGet, "/v1/nodes", tt.authHeader, []Role{RoleControlPlane}, nil)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
			}
			if called {
				t.Error("handler should not be called")
			}
			if tt.wantBody != "" && !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Errorf("body %q missing %q", rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAuthorizerBearerToken_ValidToken(t *testing.T) {
	token := mustGenerateAPIToken(t, string(RoleControlPlane), time.Now().Add(24*time.Hour))
	mq := &mockQuerier{}
	h := newBearerHarness(t, Options{TokenSecret: testSecret, Querier: mq})

	rr, called := h.do(http.MethodGet, "/v1/nodes", "Bearer "+token, []Role{RoleControlPlane},
		func(w http.ResponseWriter, r *http.Request) {
			identity, ok := IdentityFromContext(r.Context())
			if !ok {
				t.Error("expected identity in context")
			}
			if identity.Role != RoleControlPlane {
				t.Errorf("Role=%s want %s", identity.Role, RoleControlPlane)
			}
			w.WriteHeader(http.StatusOK)
		})

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !called {
		t.Error("handler was not called")
	}

	time.Sleep(50 * time.Millisecond)
	if !mq.APITokenLastUsedCalled() {
		t.Error("expected UpdateAPITokenLastUsed to be called")
	}
}

func TestAuthorizerBearerToken_RoleExtraction(t *testing.T) {
	expiresAt := time.Now().Add(24 * time.Hour)

	tests := []struct {
		role         Role
		allowedRoles []Role
		wantAllowed  bool
	}{
		{RoleControlPlane, []Role{RoleControlPlane}, true},
		{RoleWorker, []Role{RoleWorker}, true},
		{RoleCLIAdmin, []Role{RoleCLIAdmin}, true},
		{RoleCLIAdmin, []Role{RoleControlPlane}, true},  // admin can access control-plane
		{RoleControlPlane, []Role{RoleWorker}, false},    // control-plane cannot access worker
		{RoleWorker, []Role{RoleCLIAdmin}, false},        // worker cannot access admin
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_accessing_%v", tt.role, tt.allowedRoles), func(t *testing.T) {
			token := mustGenerateAPIToken(t, string(tt.role), expiresAt)
			h := newBearerHarness(t, Options{TokenSecret: testSecret, Querier: &mockQuerier{}})

			rr, _ := h.do(http.MethodGet, "/v1/test", "Bearer "+token, tt.allowedRoles, nil)

			if tt.wantAllowed {
				if rr.Code != http.StatusOK {
					t.Errorf("expected allowed, got status %d", rr.Code)
				}
			} else {
				if rr.Code != http.StatusForbidden {
					t.Errorf("expected forbidden, got status %d", rr.Code)
				}
			}
		})
	}
}

func TestAuthorizerBearerToken_BootstrapToken(t *testing.T) {
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	tokenString, err := GenerateBootstrapToken(testSecret, testClusterID, nodeID, time.Now().Add(15*time.Minute))
	if err != nil {
		t.Fatalf("GenerateBootstrapToken: %v", err)
	}

	mq := &mockQuerier{}
	auth := NewAuthorizer(Options{TokenSecret: testSecret, Querier: mq})

	req := httptest.NewRequest(http.MethodPost, "/v1/pki/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	req.Header.Set("PLOY_NODE_UUID", nodeID.String())
	rr := httptest.NewRecorder()

	called := false
	auth.Middleware(RoleWorker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Error("expected identity in context")
		}
		if identity.Role != RoleWorker {
			t.Errorf("Role=%s want %s", identity.Role, RoleWorker)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
	if !called {
		t.Error("handler was not called")
	}

	time.Sleep(50 * time.Millisecond)
	if !mq.BootstrapTokenLastUsedCalled() {
		t.Error("expected UpdateBootstrapTokenLastUsed to be called")
	}
}

func TestAuthorizerBearerToken_NoQuerierConfigured(t *testing.T) {
	token := mustGenerateAPIToken(t, string(RoleControlPlane), time.Now().Add(24*time.Hour))
	h := newBearerHarness(t, Options{TokenSecret: testSecret})

	rr, _ := h.do(http.MethodGet, "/v1/nodes", "Bearer "+token, []Role{RoleControlPlane}, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthorizerBearerToken_InsecureModeWithToken(t *testing.T) {
	token := mustGenerateAPIToken(t, string(RoleControlPlane), time.Now().Add(24*time.Hour))
	h := newBearerHarness(t, Options{
		AllowInsecure: true,
		DefaultRole:   RoleWorker,
		TokenSecret:   testSecret,
	})

	rr, _ := h.do(http.MethodGet, "/v1/nodes", "Bearer "+token, []Role{RoleControlPlane},
		func(w http.ResponseWriter, r *http.Request) {
			identity, ok := IdentityFromContext(r.Context())
			if !ok {
				t.Error("expected identity in context")
			}
			if identity.Role != RoleControlPlane {
				t.Errorf("Role=%s want %s (should use token role, not DefaultRole)", identity.Role, RoleControlPlane)
			}
			w.WriteHeader(http.StatusOK)
		})

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthorizerQueryToken(t *testing.T) {
	token := mustGenerateAPIToken(t, string(RoleControlPlane), time.Now().Add(24*time.Hour))
	path := "/v1/runs/run-123/logs?auth_token=" + token

	tests := []struct {
		name             string
		method           string
		wrapQueryAllowed bool
		wantCode         int
	}{
		{"GET with flag allowed", http.MethodGet, true, http.StatusOK},
		{"GET without flag rejected", http.MethodGet, false, http.StatusUnauthorized},
		{"POST without flag rejected", http.MethodPost, false, http.StatusUnauthorized},
		{"POST with flag still rejected", http.MethodPost, true, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAuthorizer(Options{
				TokenSecret: testSecret,
				Querier:     &mockQuerier{},
			})

			inner := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			var handler http.Handler = inner
			if tt.wrapQueryAllowed {
				handler = WithQueryTokenAllowed(inner)
			}

			req := httptest.NewRequest(tt.method, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantCode)
			}
		})
	}
}
