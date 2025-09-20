package build

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	envstore "github.com/iw2rmb/ploy/internal/envstore"
	mem "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

// minimal EnvStore that returns empty config without error
type noopEnvStore struct{}

func (n *noopEnvStore) Get(app, key string) (string, bool, error) { return "", false, nil }
func (n *noopEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	return envstore.AppEnvVars{}, nil
}
func (n *noopEnvStore) Set(app, key, value string) error                     { return nil }
func (n *noopEnvStore) SetAll(app string, envVars envstore.AppEnvVars) error { return nil }
func (n *noopEnvStore) Get2(app, key string) (string, bool, error)           { return "", false, nil }
func (n *noopEnvStore) Delete(app, key string) error                         { return nil }
func (n *noopEnvStore) ToStringArray(app string) ([]string, error)           { return []string{}, nil }

func TestTriggerWrappers_InvalidAppNameFastFail(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*fiber.Ctx) error
	}{
		{"TriggerBuild", func(c *fiber.Ctx) error { return TriggerBuild(c, nil, &noopEnvStore{}) }},
		{"TriggerBuildWithContext", func(c *fiber.Ctx) error { return TriggerBuildWithContext(c, nil, &noopEnvStore{}, "apps") }},
		{"TriggerPlatformBuild", func(c *fiber.Ctx) error { return TriggerPlatformBuild(c, nil, &noopEnvStore{}) }},
		{"TriggerAppBuild", func(c *fiber.Ctx) error { return TriggerAppBuild(c, nil, &noopEnvStore{}) }},
		{"TriggerBuildWithStorage", func(c *fiber.Ctx) error { return TriggerBuildWithStorage(c, mem.NewMemoryStorage(0), &noopEnvStore{}) }},
		{"TriggerPlatformBuildWithStorage", func(c *fiber.Ctx) error {
			return TriggerPlatformBuildWithStorage(c, mem.NewMemoryStorage(0), &noopEnvStore{})
		}},
		{"TriggerAppBuildWithStorage", func(c *fiber.Ctx) error {
			return TriggerAppBuildWithStorage(c, mem.NewMemoryStorage(0), &noopEnvStore{})
		}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Post("/build/:app", tt.handler)

			// invalid app name causes early 400 without invoking builders
			req := httptest.NewRequest("POST", "/build/"+url.PathEscape("invalid app"), bytes.NewReader([]byte("dummy")))
			req.Header.Set("Content-Type", "application/x-tar")
			resp, err := app.Test(req, 10000)
			require.NoError(t, err)
			assert.Equal(t, 400, resp.StatusCode)
			require.NoError(t, resp.Body.Close())
		})
	}
}
