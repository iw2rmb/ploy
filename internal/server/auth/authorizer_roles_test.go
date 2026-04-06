package auth

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
)

func TestNormalizeRoleAndAllowlist(t *testing.T) {
	cases := map[string]Role{
		"control":       RoleControlPlane,
		"Control-Plane": RoleControlPlane,
		"worker":        RoleWorker,
		"admin":         RoleCLIAdmin,
		"cliadmin":      RoleCLIAdmin,
		"custom":        "", // Unknown roles now return empty
	}
	for in, want := range cases {
		if got := NormalizeRole(in); got != want {
			t.Fatalf("NormalizeRole(%q)=%q want %q", in, got, want)
		}
	}

	if allowlist(nil) != nil {
		t.Fatal("allowlist(nil) should return nil")
	}
	al := allowlist([]Role{RoleWorker, RoleControlPlane})
	if _, ok := al[RoleWorker]; !ok {
		t.Fatal("allowlist should contain worker")
	}
	if _, ok := al[RoleControlPlane]; !ok {
		t.Fatal("allowlist should contain control-plane")
	}
}

func TestExtractRoleFromCertOUAndCN(t *testing.T) {
	tests := []struct {
		name string
		cert *x509.Certificate
		want Role
	}{
		{
			name: "OU=worker",
			cert: &x509.Certificate{Subject: pkix.Name{OrganizationalUnit: []string{"Ploy role=worker"}}},
			want: RoleWorker,
		},
		{
			name: "CN fallback control-*",
			cert: &x509.Certificate{Subject: pkix.Name{CommonName: "control-abc"}},
			want: RoleControlPlane,
		},
		{
			name: "CN node:<node_id> maps to worker",
			cert: &x509.Certificate{Subject: pkix.Name{CommonName: "node:aB3xY9"}},
			want: RoleWorker,
		},
		{
			name: "nil cert returns empty",
			cert: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractRole(tt.cert); got != tt.want {
				t.Fatalf("extractRole got %q, want %q", got, tt.want)
			}
		})
	}
}
