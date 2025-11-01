package auth

import (
    "context"
    "crypto/x509"
    "crypto/x509/pkix"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestNormalizeRoleAndAllowlist(t *testing.T) {
    cases := map[string]string{
        "control":       RoleControlPlane,
        "Control-Plane": RoleControlPlane,
        "worker":        RoleWorker,
        "admin":         RoleCLIAdmin,
        "cliadmin":      RoleCLIAdmin,
        "custom":        "custom",
    }
    for in, want := range cases {
        if got := normalizeRole(in); got != want {
            t.Fatalf("normalizeRole(%q)=%q want %q", in, got, want)
        }
    }

    if allowlist(nil) != nil {
        t.Fatal("allowlist(nil) should return nil")
    }
    al := allowlist([]string{"WORKER", "controlplane", "unknown"})
    if _, ok := al[RoleWorker]; !ok {
        t.Fatal("allowlist should contain worker")
    }
    if _, ok := al[RoleControlPlane]; !ok {
        t.Fatal("allowlist should contain control-plane")
    }
    if _, ok := al["unknown"]; !ok {
        t.Fatal("allowlist should retain unknown role")
    }
}

func TestExtractRoleFromCertOUAndCN(t *testing.T) {
    cert := &x509.Certificate{Subject: pkix.Name{OrganizationalUnit: []string{"Ploy role=worker"}}}
    if got := extractRole(cert); got != RoleWorker {
        t.Fatalf("extractRole OU=worker got %q", got)
    }
    cert = &x509.Certificate{Subject: pkix.Name{CommonName: "control-abc"}}
    if got := extractRole(cert); got != RoleControlPlane {
        t.Fatalf("extractRole CN fallback got %q", got)
    }
    if got := extractRole(nil); got != "" {
        t.Fatalf("extractRole nil cert got %q", got)
    }
}

func TestMiddlewareNilNextReturns404(t *testing.T) {
    a := NewAuthorizer(Options{AllowInsecure: true, DefaultRole: RoleControlPlane})
    h := a.Middleware()(nil)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    h.ServeHTTP(rr, req)
    if rr.Code != http.StatusNotFound {
        t.Fatalf("expected 404, got %d", rr.Code)
    }
}

func TestIdentityFromContextNone(t *testing.T) {
    if _, ok := IdentityFromContext(context.TODO()); ok {
        t.Fatalf("expected no identity for nil context")
    }
}

