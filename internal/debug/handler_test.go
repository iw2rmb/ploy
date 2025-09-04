package debug

import (
    "encoding/json"
    "net/http/httptest"
    "testing"
    "strings"

	"github.com/gofiber/fiber/v2"
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
	"github.com/iw2rmb/ploy/internal/envstore"
	ipolicy "github.com/iw2rmb/ploy/internal/policy"
)

type fakeEnvStore struct{}

func (fakeEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	return envstore.AppEnvVars{"A": "1"}, nil
}
func (fakeEnvStore) SetAll(app string, vars envstore.AppEnvVars) error { return nil }
func (fakeEnvStore) Delete(app, key string) error                      { return nil }
func (fakeEnvStore) Get(app, key string) (string, bool, error)         { return "", false, nil }
func (fakeEnvStore) Set(app, key, value string) error                  { return nil }
func (fakeEnvStore) ToStringArray(app string) ([]string, error)        { return []string{}, nil }

type recordingEnforcer struct {
	called bool
	last   ipolicy.ArtifactInput
}

func (r *recordingEnforcer) Enforce(in ipolicy.ArtifactInput) error {
	r.called = true
	r.last = in
	return nil
}

type recordingBuilder struct{ called bool }

func (r *recordingBuilder) BuildDebugInstance(app, lane, srcDir, sha, outDir string, envVars map[string]string, sshEnabled bool) (*ibuilders.DebugBuildResult, error) {
	r.called = true
	return &ibuilders.DebugBuildResult{SSHCommand: "ssh test", SSHPublicKey: "key", ImagePath: "/tmp/out.img"}, nil
}

func TestDebugApp_UsesPolicyAndBuilder(t *testing.T) {
	// Inject fakes
	recEnf := &recordingEnforcer{}
	ipolicy.DefaultEnforcer = recEnf
	recBld := &recordingBuilder{}
	ibuilders.DefaultDebugBuilder = recBld

	app := fiber.New()
	app.Post("/debug/:app", func(c *fiber.Ctx) error { return DebugApp(c, fakeEnvStore{}) })

    req := httptest.NewRequest("POST", "/debug/myapp?lane=c&env=dev", strings.NewReader("{}"))
    req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request err: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !recEnf.called {
		t.Fatalf("policy enforcer not called")
	}
	if !recBld.called {
		t.Fatalf("debug builder not called")
	}
	// verify JSON body
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if body["status"] != "debug_created" {
		t.Fatalf("unexpected status: %v", body["status"])
	}
}
