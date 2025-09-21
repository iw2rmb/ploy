package sync_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/routing"
	"github.com/iw2rmb/ploy/internal/routing/sync"
)

func TestGenerateDynamicConfigAddsRoutersAndPreservesMiddlewares(t *testing.T) {
	baseYAML := []byte(`http:
  middlewares:
    https-redirect:
      redirectScheme:
        scheme: https
  routers: {}
tls:
  options:
    default:
      minVersion: VersionTLS12
`)

	routes := map[string]map[string]routing.DomainRoute{
		"demo": {
			"demo.example": {
				App:        "demo",
				Domain:     "demo.example",
				AllocID:    "alloc-1",
				AllocIP:    "10.0.0.5",
				Port:       8080,
				HealthPath: "/healthz",
				Aliases:    []string{"www.demo.example"},
				TLSEnabled: true,
			},
		},
	}

	generated, err := sync.GenerateDynamicConfig(baseYAML, routes)
	require.NoError(t, err)

	var cfg sync.DynamicConfig
	require.NoError(t, yaml.Unmarshal(generated, &cfg))
	require.NotNil(t, cfg.HTTP)
	require.Contains(t, cfg.HTTP.Middlewares, "https-redirect")

	gotRouter, ok := cfg.HTTP.Routers["demo--demo-example"]
	require.True(t, ok)
	require.Equal(t, "Host(`demo.example`,`www.demo.example`)", gotRouter.Rule)
	require.Contains(t, gotRouter.EntryPoints, "websecure")
	require.Equal(t, "demo@consulcatalog", gotRouter.Service)
	require.NotNil(t, gotRouter.TLS)
	require.Equal(t, "default-acme", gotRouter.TLS.CertResolver)
	require.Contains(t, gotRouter.Middlewares, "secure-headers@file")
}
