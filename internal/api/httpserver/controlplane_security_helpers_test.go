package httpserver_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
)

func newTestControlPlaneHandler(t *testing.T, opts httpserver.ControlPlaneOptions) http.Handler {
	t.Helper()
	if opts.Authorizer == nil {
		opts.Authorizer = auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
	}
	return httpserver.NewControlPlaneHandler(opts)
}

func newTestPrincipal(scopes []string) security.Principal {
	now := time.Now().UTC()
	return security.Principal{
		SecretName: "test-client",
		TokenID:    "token-123",
		Scopes:     scopes,
		IssuedAt:   now,
		ExpiresAt:  now.Add(time.Hour),
	}
}

func newMTLSRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			Subject: pkix.Name{
				CommonName:         "control-plane-test",
				OrganizationalUnit: []string{"Ploy control-plane"},
			},
		}},
	}
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

type testTokenVerifier struct {
	principal security.Principal
	err       error
}

func (t *testTokenVerifier) Verify(ctx context.Context, token string) (security.Principal, error) {
	if t.err != nil {
		return security.Principal{}, t.err
	}
	return t.principal, nil
}
