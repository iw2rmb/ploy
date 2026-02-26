package auth

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// mockQuerier embeds a nil pointer to satisfy the interface with defaults,
// then overrides only the token-related methods needed for testing.
type mockQuerier struct {
	*store.Queries // Embedded nil pointer provides default panic behavior for unused methods

	mu sync.Mutex

	checkAPITokenRevokedFunc         func(ctx context.Context, tokenID string) (pgtype.Timestamptz, error)
	checkBootstrapTokenRevokedFunc   func(ctx context.Context, tokenID string) (pgtype.Timestamptz, error)
	updateAPITokenLastUsedFunc       func(ctx context.Context, tokenID string) error
	updateBootstrapTokenLastUsedFunc func(ctx context.Context, tokenID string) error

	// Track calls for verification
	updateAPITokenLastUsedCalled       bool
	updateBootstrapTokenLastUsedCalled bool
}

func (m *mockQuerier) CheckAPITokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	if m.checkAPITokenRevokedFunc != nil {
		return m.checkAPITokenRevokedFunc(ctx, tokenID)
	}
	// Default: token not revoked (return no rows error)
	return pgtype.Timestamptz{}, sql.ErrNoRows
}

func (m *mockQuerier) CheckBootstrapTokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	if m.checkBootstrapTokenRevokedFunc != nil {
		return m.checkBootstrapTokenRevokedFunc(ctx, tokenID)
	}
	// Default: token not revoked
	return pgtype.Timestamptz{}, sql.ErrNoRows
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

func TestAuthorizerBearerToken_ValidToken(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	// Generate valid token
	tokenString, err := GenerateAPIToken(secret, clusterID, string(role), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Create authorizer with mock querier
	mockQ := &mockQuerier{}
	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret,
		Querier:       mockQ,
	})

	// Create request with bearer token
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	// Test middleware
	rr := httptest.NewRecorder()
	called := false
	handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		// Verify identity is set in context
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Error("expected identity in context")
		}
		if identity.Role != role {
			t.Errorf("Role=%s want %s", identity.Role, role)
		}

		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !called {
		t.Error("handler was not called")
	}

	// Give async updateTokenLastUsed a moment to run
	time.Sleep(50 * time.Millisecond)
	if !mockQ.APITokenLastUsedCalled() {
		t.Error("expected UpdateAPITokenLastUsed to be called")
	}
}

func TestAuthorizerBearerToken_ExpiredToken(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(-1 * time.Hour) // Already expired

	// Generate expired token
	tokenString, err := GenerateAPIToken(secret, clusterID, string(role), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	mockQ := &mockQuerier{}
	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret,
		Querier:       mockQ,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	rr := httptest.NewRecorder()
	handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for expired token")
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestAuthorizerBearerToken_RevokedToken(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	tokenString, err := GenerateAPIToken(secret, clusterID, string(role), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Mock querier that returns a revocation timestamp
	mockQ := &mockQuerier{
		checkAPITokenRevokedFunc: func(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
			// Return a timestamp indicating the token was revoked
			return pgtype.Timestamptz{Time: time.Now(), Valid: true}, nil
		},
	}

	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret,
		Querier:       mockQ,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	rr := httptest.NewRecorder()
	handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for revoked token")
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
	if !contains(rr.Body.String(), "revoked") {
		t.Errorf("expected error message to mention 'revoked', got: %s", rr.Body.String())
	}
}

func TestAuthorizerBearerToken_MissingToken(t *testing.T) {
	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   "test-secret",
	})

	// Request without Authorization header
	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	rr := httptest.NewRecorder()

	handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without token")
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestAuthorizerBearerToken_InvalidFormat(t *testing.T) {
	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   "test-secret",
	})

	tests := []struct {
		name   string
		header string
	}{
		{"malformed token", "Bearer invalid-token"},
		{"missing Bearer prefix", "invalid-token"},
		{"empty Bearer", "Bearer "},
		{"wrong prefix", "Basic base64encodedcreds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
			req.Header.Set("Authorization", tt.header)
			rr := httptest.NewRecorder()

			handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("handler should not be called for invalid token")
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("got status %d, want %d for %s", rr.Code, http.StatusForbidden, tt.name)
			}
		})
	}
}

func TestAuthorizerBearerToken_RoleExtraction(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	expiresAt := time.Now().Add(24 * time.Hour)

	tests := []struct {
		role         Role
		allowedRoles []Role
		wantAllowed  bool
	}{
		{RoleControlPlane, []Role{RoleControlPlane}, true},
		{RoleWorker, []Role{RoleWorker}, true},
		{RoleCLIAdmin, []Role{RoleCLIAdmin}, true},
		{RoleCLIAdmin, []Role{RoleControlPlane}, true}, // Admin can access control-plane endpoints
		{RoleControlPlane, []Role{RoleWorker}, false},  // control-plane cannot access worker endpoints
		{RoleWorker, []Role{RoleCLIAdmin}, false},      // worker cannot access admin endpoints
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_accessing_%v", tt.role, tt.allowedRoles), func(t *testing.T) {
			tokenString, err := GenerateAPIToken(secret, clusterID, string(tt.role), expiresAt)
			if err != nil {
				t.Fatalf("GenerateAPIToken error: %v", err)
			}

			mockQ := &mockQuerier{}
			auth := NewAuthorizer(Options{
				AllowInsecure: false,
				TokenSecret:   secret,
				Querier:       mockQ,
			})

			req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
			req.Header.Set("Authorization", "Bearer "+tokenString)
			rr := httptest.NewRecorder()

			handler := auth.Middleware(tt.allowedRoles...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

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
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	expiresAt := time.Now().Add(15 * time.Minute)

	// Generate bootstrap token
	tokenString, err := GenerateBootstrapToken(secret, clusterID, nodeID, expiresAt)
	if err != nil {
		t.Fatalf("GenerateBootstrapToken error: %v", err)
	}

	mockQ := &mockQuerier{}
	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret,
		Querier:       mockQ,
	})

	// Bootstrap token should work for worker endpoints
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	req.Header.Set("PLOY_NODE_UUID", nodeID.String())
	rr := httptest.NewRecorder()

	handler := auth.Middleware(RoleWorker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Error("expected identity in context")
		}
		if identity.Role != RoleWorker {
			t.Errorf("Role=%s want %s", identity.Role, RoleWorker)
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	// Give async updateTokenLastUsed a moment to run
	time.Sleep(50 * time.Millisecond)
	if !mockQ.BootstrapTokenLastUsedCalled() {
		t.Error("expected UpdateBootstrapTokenLastUsed to be called")
	}
}

func TestAuthorizerBearerToken_NoQuerierConfigured(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	tokenString, err := GenerateAPIToken(secret, clusterID, string(role), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Authorizer without querier (no database)
	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret,
		Querier:       nil, // No database
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rr := httptest.NewRecorder()

	handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	// Should still work - tokens just can't be revoked without database
	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthorizerBearerToken_WrongSecret(t *testing.T) {
	secret1 := "secret-one-12345678901234567890"
	secret2 := "secret-two-12345678901234567890"
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	// Generate token with secret1
	tokenString, err := GenerateAPIToken(secret1, clusterID, string(role), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Try to validate with secret2
	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret2, // Different secret
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rr := httptest.NewRecorder()

	handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for token with wrong secret")
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestAuthorizerBearerToken_InsecureModeWithToken(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	tokenString, err := GenerateAPIToken(secret, clusterID, string(role), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Insecure mode still validates bearer tokens if provided
	auth := NewAuthorizer(Options{
		AllowInsecure: true,
		DefaultRole:   RoleWorker,
		TokenSecret:   secret,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rr := httptest.NewRecorder()

	handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Error("expected identity in context")
		}
		// Should use role from token, not DefaultRole
		if identity.Role != RoleControlPlane {
			t.Errorf("Role=%s want %s", identity.Role, RoleControlPlane)
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthorizerQueryToken_AllowedArtifactPaths(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	expiresAt := time.Now().Add(24 * time.Hour)
	tokenString, err := GenerateAPIToken(secret, clusterID, string(RoleControlPlane), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret,
		Querier:       &mockQuerier{},
	})

	tests := []string{
		"/v1/runs/run-123/logs?auth_token=" + tokenString,
		"/v1/runs/run-123/repos/repo-123/logs?auth_token=" + tokenString,
		"/v1/runs/run-123/repos/repo-123/diffs?download=true&diff_id=diff-1&auth_token=" + tokenString,
	}

	for _, path := range tests {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("path %s: got status %d, want %d; body=%q", path, rr.Code, http.StatusOK, rr.Body.String())
		}
	}
}

func TestAuthorizerQueryToken_DisallowedPathOrMethod(t *testing.T) {
	secret := "test-secret-key-for-jwt-signing-at-least-32-chars"
	clusterID := "test-cluster"
	expiresAt := time.Now().Add(24 * time.Hour)
	tokenString, err := GenerateAPIToken(secret, clusterID, string(RoleControlPlane), expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	auth := NewAuthorizer(Options{
		AllowInsecure: false,
		TokenSecret:   secret,
		Querier:       &mockQuerier{},
	})

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/runs/run-123?auth_token=" + tokenString},
		{http.MethodPost, "/v1/runs/run-123/logs?auth_token=" + tokenString},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rr := httptest.NewRecorder()
		handler := auth.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s %s: got status %d, want %d", tt.method, tt.path, rr.Code, http.StatusForbidden)
		}
	}
}
