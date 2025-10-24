package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddlewareRejectsNonTLS(t *testing.T) {
	t.Helper()

	m := NewManager(&stubVerifier{
		principal: Principal{
			SecretName: "gitlab-admin",
			Scopes:     []string{"api"},
			IssuedAt:   time.Now().UTC(),
			ExpiresAt:  time.Now().Add(time.Hour).UTC(),
		},
	})

	handler := m.Middleware("api")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-TLS request, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsMissingAuthorization(t *testing.T) {
	t.Helper()

	m := NewManager(&stubVerifier{})
	handler := m.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
	}
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing authorization, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsInvalidToken(t *testing.T) {
	t.Helper()

	m := NewManager(&stubVerifier{
		err: errors.New("invalid token"),
	})

	handler := m.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
	}
	req.Header.Set("Authorization", "Bearer broken")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", rec.Code)
	}
}

func TestMiddlewareRejectsMissingScope(t *testing.T) {
	t.Helper()

	m := NewManager(&stubVerifier{
		principal: Principal{
			SecretName: "gitlab-ci",
			Scopes:     []string{"read_repository"},
			IssuedAt:   time.Now().UTC(),
			ExpiresAt:  time.Now().Add(time.Minute).UTC(),
		},
	})

	handler := m.Middleware("write_repository")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
	}
	req.Header.Set("Authorization", "Bearer scope-missing")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing scope, got %d", rec.Code)
	}
}

func TestMiddlewareInjectsPrincipalIntoContext(t *testing.T) {
	t.Helper()

	now := time.Date(2025, time.October, 24, 8, 0, 0, 0, time.UTC)
	principal := Principal{
		SecretName: "gitlab-ci",
		TokenID:    "token-abc",
		Scopes:     []string{"api", "write_repository"},
		IssuedAt:   now,
		ExpiresAt:  now.Add(30 * time.Minute),
	}
	verifier := &stubVerifier{
		principal: principal,
	}

	m := NewManager(verifier)
	handler := m.Middleware("write_repository")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatalf("expected principal in context")
		}
		if got.TokenID != principal.TokenID {
			t.Fatalf("expected token id %s, got %s", principal.TokenID, got.TokenID)
		}
		if !got.HasScope("api") {
			t.Fatalf("expected api scope present")
		}
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
	}
	req.Header.Set("Authorization", "Bearer token-abc")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected handler status 202, got %d", rec.Code)
	}
	if verifier.lastToken != "token-abc" {
		t.Fatalf("expected verifier to see token token-abc, got %s", verifier.lastToken)
	}
}

type stubVerifier struct {
	principal Principal
	err       error
	lastToken string
}

func (s *stubVerifier) Verify(_ context.Context, token string) (Principal, error) {
	s.lastToken = token
	if s.err != nil {
		return Principal{}, s.err
	}
	return s.principal, nil
}
