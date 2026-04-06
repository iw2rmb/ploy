package server

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// newTestServer creates an HTTPServer with insecure auth on an OS-assigned port.
func newTestServer(t *testing.T, cfg ...config.HTTPConfig) *HTTPServer {
	t.Helper()
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleControlPlane,
	})
	var httpCfg config.HTTPConfig
	if len(cfg) > 0 {
		httpCfg = cfg[0]
	} else {
		httpCfg = config.HTTPConfig{Listen: "127.0.0.1:0"}
	}
	srv, err := NewHTTPServer(HTTPOptions{
		Config:     httpCfg,
		Authorizer: authorizer,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer() error = %v", err)
	}
	return srv
}
