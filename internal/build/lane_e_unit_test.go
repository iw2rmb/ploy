package build

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	project "github.com/iw2rmb/ploy/internal/detect/project"
	"github.com/iw2rmb/ploy/internal/storage"
	mem "github.com/iw2rmb/ploy/internal/storage/providers/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ensure buildLaneE returns 400 when Dockerfile is missing and autogen is not enabled.
func TestBuildLaneE_MissingDockerfile_NoAutogen_400(t *testing.T) {
	tmp := t.TempDir()
	deps := &BuildDependencies{}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false}

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, _, _, _ = buildLaneE(c, deps, buildCtx, "app", tmp, "sha", tmp, "", facts, map[string]string{})
		// Let buildLaneE write the response status/body
		return nil
	})
	req := httptest.NewRequest("POST", "/e?autogen_dockerfile=false", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	require.Equal(t, 400, resp.StatusCode)
}

func TestBuildLaneE_JibSuccess(t *testing.T) {
	tmp := t.TempDir()
	deps := &BuildDependencies{}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: true}

	// Stub ociBuilder
	origOCI := ociBuilder
	ociBuilder = func(appName, srcDir, tag string, envVars map[string]string) (string, error) {
		// Ensure tag looks like a registry ref
		if !strings.Contains(tag, appName) {
			return "", fmt.Errorf("unexpected tag: %s", tag)
		}
		return "registry.dev/example/" + appName + ":sha", nil
	}
	t.Cleanup(func() { ociBuilder = origOCI })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		imgPath, dockerImage, job, err := buildLaneE(c, deps, buildCtx, "app", tmp, "sha", tmp, "", facts, map[string]string{})
		require.NoError(t, err)
		return c.JSON(fiber.Map{"imagePath": imgPath, "dockerImage": dockerImage, "job": job})
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e", nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
}

func TestBuildLaneE_JibPrereqMissing_Returns400(t *testing.T) {
	tmp := t.TempDir()
	deps := &BuildDependencies{}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: true}

	// Stub ociBuilder to simulate prerequisites missing
	origOCI := ociBuilder
	ociBuilder = func(appName, srcDir, tag string, envVars map[string]string) (string, error) {
		return "", fmt.Errorf("no Dockerfile or Jib configuration present")
	}
	t.Cleanup(func() { ociBuilder = origOCI })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, _, _, _ = buildLaneE(c, deps, buildCtx, "app", tmp, "sha", tmp, "", facts, map[string]string{})
		return nil
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e", nil))
	require.NoError(t, err)
	require.Equal(t, 400, resp.StatusCode)
}

func TestBuildLaneE_KanikoHappyPath(t *testing.T) {
	// Prepare source dir with a Dockerfile to skip autogen
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "Dockerfile"), []byte("FROM scratch\n"), 0644))
	deps := &BuildDependencies{Storage: mem.NewMemoryStorage(0)}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false}

	// Stub seams
	origUpload := uploadWithUnifiedStorage
	uploadWithUnifiedStorage = func(ctx context.Context, _ storage.Storage, filePath, storageKey, contentType string) error {
		// basic sanity of args
		if !strings.HasSuffix(filePath, ".tar") || !strings.Contains(storageKey, "/src.tar") {
			return fmt.Errorf("unexpected upload args: %s %s", filePath, storageKey)
		}
		return nil
	}
	t.Cleanup(func() { uploadWithUnifiedStorage = origUpload })

	// Local HTTP server to accept the mirror PUT and avoid sleeps
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(405)
	}))
	defer srv.Close()
	t.Setenv("PLOY_SEAWEEDFS_URL", srv.URL)

	origRender := renderKanikoBuilderFn
	renderKanikoBuilderFn = func(app, version, tag, contextURL, dockerfile, language string) (string, error) {
		f, err := os.CreateTemp("", "builder-*.hcl")
		if err != nil {
			return "", err
		}
		_ = f.Close()
		return f.Name(), nil
	}
	t.Cleanup(func() { renderKanikoBuilderFn = origRender })

	origValidate := validateJobFn
	validateJobFn = func(path string) error { return nil }
	t.Cleanup(func() { validateJobFn = origValidate })

	origSubmit := submitAndWaitFn
	submitAndWaitFn = func(job string, d time.Duration) error { return nil }
	t.Cleanup(func() { submitAndWaitFn = origSubmit })

	origVerify := verifyOCIPushFn
	verifyOCIPushFn = func(tag string) verifyResult { return verifyResult{OK: true, Status: 200, Digest: "sha256:deadbeef"} }
	t.Cleanup(func() { verifyOCIPushFn = origVerify })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		imgPath, dockerImage, job, err := buildLaneE(c, deps, buildCtx, "app", src, "sha", t.TempDir(), "", facts, map[string]string{})
		require.NoError(t, err)
		assert.Equal(t, "", imgPath)
		assert.NotEmpty(t, job)
		assert.Contains(t, dockerImage, "@sha256:deadbeef")
		return c.SendStatus(200)
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e", nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
}

func TestBuildLaneE_AutogenDockerfile_Success(t *testing.T) {
	// No Dockerfile initially; enable autogen via query param
	src := t.TempDir()
	deps := &BuildDependencies{Storage: mem.NewMemoryStorage(0)}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false, Language: "java", BuildTool: "maven"}

	// Stub autogen to write a minimal Dockerfile
	origGen := dockerfileGenerator
	dockerfileGenerator = func(dir string, f project.BuildFacts) error {
		return os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM busybox\n"), 0644)
	}
	t.Cleanup(func() { dockerfileGenerator = origGen })

	// Stub upload and subsequent seams
	origUpload := uploadWithUnifiedStorage
	uploadWithUnifiedStorage = func(ctx context.Context, _ storage.Storage, filePath, storageKey, contentType string) error {
		return nil
	}
	t.Cleanup(func() { uploadWithUnifiedStorage = origUpload })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(405)
	}))
	defer srv.Close()
	t.Setenv("PLOY_SEAWEEDFS_URL", srv.URL)

	origRender := renderKanikoBuilderFn
	renderKanikoBuilderFn = func(app, version, tag, contextURL, dockerfile, language string) (string, error) {
		f, _ := os.CreateTemp("", "builder-*.hcl")
		_ = f.Close()
		return f.Name(), nil
	}
	t.Cleanup(func() { renderKanikoBuilderFn = origRender })

	origValidate := validateJobFn
	validateJobFn = func(path string) error { return nil }
	t.Cleanup(func() { validateJobFn = origValidate })

	origSubmit := submitAndWaitFn
	submitAndWaitFn = func(job string, d time.Duration) error { return nil }
	t.Cleanup(func() { submitAndWaitFn = origSubmit })

	origVerify := verifyOCIPushFn
	verifyOCIPushFn = func(tag string) verifyResult { return verifyResult{OK: true, Status: 200, Digest: "sha256:cafebabe"} }
	t.Cleanup(func() { verifyOCIPushFn = origVerify })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, dockerImage, job, err := buildLaneE(c, deps, buildCtx, "app", src, "sha", t.TempDir(), "java", facts, map[string]string{})
		require.NoError(t, err)
		assert.NotEmpty(t, job)
		assert.Contains(t, dockerImage, "@sha256:cafebabe")
		return c.SendStatus(200)
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e?autogen_dockerfile=true", nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
}

func TestBuildLaneE_UploadContextFailure_500(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "Dockerfile"), []byte("FROM scratch\n"), 0644))
	deps := &BuildDependencies{Storage: mem.NewMemoryStorage(0)}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false}

	// Fail upload
	origUpload := uploadWithUnifiedStorage
	uploadWithUnifiedStorage = func(ctx context.Context, _ storage.Storage, filePath, storageKey, contentType string) error {
		return fmt.Errorf("upload failed")
	}
	t.Cleanup(func() { uploadWithUnifiedStorage = origUpload })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, _, _, _ = buildLaneE(c, deps, buildCtx, "app", src, "sha", t.TempDir(), "", facts, map[string]string{})
		return nil
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e", nil))
	require.NoError(t, err)
	require.Equal(t, 500, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "upload_context", body["stage"])
}

func TestBuildLaneE_SubmitFailure_500_WithBuilderHeader(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "Dockerfile"), []byte("FROM scratch\n"), 0644))
	deps := &BuildDependencies{Storage: mem.NewMemoryStorage(0)}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false}

	// Success upload
	origUpload := uploadWithUnifiedStorage
	uploadWithUnifiedStorage = func(ctx context.Context, _ storage.Storage, filePath, storageKey, contentType string) error {
		return nil
	}
	t.Cleanup(func() { uploadWithUnifiedStorage = origUpload })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(405)
	}))
	defer srv.Close()
	t.Setenv("PLOY_SEAWEEDFS_URL", srv.URL)

	origRender := renderKanikoBuilderFn
	renderKanikoBuilderFn = func(app, version, tag, contextURL, dockerfile, language string) (string, error) {
		f, _ := os.CreateTemp("", "builder-*.hcl")
		_ = f.Close()
		return f.Name(), nil
	}
	t.Cleanup(func() { renderKanikoBuilderFn = origRender })

	origValidate := validateJobFn
	validateJobFn = func(path string) error { return nil }
	t.Cleanup(func() { validateJobFn = origValidate })

	// Fail submit
	origSubmit := submitAndWaitFn
	submitAndWaitFn = func(job string, d time.Duration) error { return fmt.Errorf("submit failed") }
	t.Cleanup(func() { submitAndWaitFn = origSubmit })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, _, _, _ = buildLaneE(c, deps, buildCtx, "app", src, "sha", t.TempDir(), "", facts, map[string]string{})
		return nil
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e", nil))
	require.NoError(t, err)
	require.Equal(t, 500, resp.StatusCode)
	// Header should carry deployment ID
	depID := resp.Header.Get("X-Deployment-ID")
	assert.NotEmpty(t, depID)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "kaniko_submit", body["stage"])
	// builder object present
	b, ok := body["builder"].(map[string]any)
	require.True(t, ok)
	_, hasJob := b["job"]
	assert.True(t, hasJob)
}

func TestBuildLaneE_VerifyPushFailure_500(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "Dockerfile"), []byte("FROM scratch\n"), 0644))
	deps := &BuildDependencies{Storage: mem.NewMemoryStorage(0)}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false}

	// Success upload
	origUpload := uploadWithUnifiedStorage
	uploadWithUnifiedStorage = func(ctx context.Context, _ storage.Storage, filePath, storageKey, contentType string) error {
		return nil
	}
	t.Cleanup(func() { uploadWithUnifiedStorage = origUpload })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(405)
	}))
	defer srv.Close()
	t.Setenv("PLOY_SEAWEEDFS_URL", srv.URL)

	origRender := renderKanikoBuilderFn
	renderKanikoBuilderFn = func(app, version, tag, contextURL, dockerfile, language string) (string, error) {
		f, _ := os.CreateTemp("", "builder-*.hcl")
		_ = f.Close()
		return f.Name(), nil
	}
	t.Cleanup(func() { renderKanikoBuilderFn = origRender })

	origValidate := validateJobFn
	validateJobFn = func(path string) error { return nil }
	t.Cleanup(func() { validateJobFn = origValidate })

	// Submit ok
	origSubmit := submitAndWaitFn
	submitAndWaitFn = func(job string, d time.Duration) error { return nil }
	t.Cleanup(func() { submitAndWaitFn = origSubmit })

	// Verify push fails
	origVerify := verifyOCIPushFn
	verifyOCIPushFn = func(tag string) verifyResult {
		return verifyResult{OK: false, Status: 502, Digest: "", Message: "gateway"}
	}
	t.Cleanup(func() { verifyOCIPushFn = origVerify })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, _, _, _ = buildLaneE(c, deps, buildCtx, "app", src, "sha", t.TempDir(), "", facts, map[string]string{})
		return nil
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e", nil))
	require.NoError(t, err)
	require.Equal(t, 500, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "verify_push", body["stage"])
	// status value surfaced
	assert.Equal(t, float64(502), body["status"]) // json numbers decode as float64
}

func TestBuildLaneE_ValidateBuilderFailure_500(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "Dockerfile"), []byte("FROM scratch\n"), 0644))
	deps := &BuildDependencies{Storage: mem.NewMemoryStorage(0)}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false}

	// Success upload
	origUpload := uploadWithUnifiedStorage
	uploadWithUnifiedStorage = func(ctx context.Context, _ storage.Storage, filePath, storageKey, contentType string) error {
		return nil
	}
	t.Cleanup(func() { uploadWithUnifiedStorage = origUpload })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(405)
	}))
	defer srv.Close()
	t.Setenv("PLOY_SEAWEEDFS_URL", srv.URL)

	origRender := renderKanikoBuilderFn
	renderKanikoBuilderFn = func(app, version, tag, contextURL, dockerfile, language string) (string, error) {
		f, _ := os.CreateTemp("", "builder-*.hcl")
		_ = f.Close()
		return f.Name(), nil
	}
	t.Cleanup(func() { renderKanikoBuilderFn = origRender })

	// Validation fails
	origValidate := validateJobFn
	validateJobFn = func(path string) error { return fmt.Errorf("validation error") }
	t.Cleanup(func() { validateJobFn = origValidate })

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, _, _, _ = buildLaneE(c, deps, buildCtx, "app", src, "sha", t.TempDir(), "", facts, map[string]string{})
		return nil
	})

	resp, err := app.Test(httptest.NewRequest("POST", "/e", nil))
	require.NoError(t, err)
	require.Equal(t, 500, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "validate_builder", body["stage"])
}
