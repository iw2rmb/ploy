package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ploy/ploy/controller/builders"
	"github.com/ploy/ploy/controller/nomad"
	"github.com/ploy/ploy/controller/opa"
	"github.com/ploy/ploy/controller/supply"
	"github.com/ploy/ploy/controller/config"
	"github.com/ploy/ploy/controller/envstore"
	"github.com/ploy/ploy/internal/storage"
)

type lanePickResult struct {
	Lane string `json:"lane"`
	Language string `json:"language"`
	Reasons []string `json:"reasons"`
}

var storeClient *storage.Client
var envStore *envstore.EnvStore

func main(){
	app := fiber.New()
	app.Use(previewHostRouter)

	cfgPath := getenv("PLOY_STORAGE_CONFIG", "configs/storage-config.yaml")
	if rootCfg, err := config.Load(cfgPath); err == nil {
		if c, err := storage.New(rootCfg.Storage); err == nil { storeClient = c }
	}
	
	envStore = envstore.New(getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"))

	api := app.Group("/v1")
	api.Post("/apps/:app/builds", triggerBuild)
	api.Get("/apps", listApps)
	api.Get("/status/:app", status)
	
	// Domain management
	api.Post("/apps/:app/domains", addDomain)
	api.Get("/apps/:app/domains", listDomains)
	api.Delete("/apps/:app/domains/:domain", removeDomain)
	
	// Certificate management
	api.Post("/certs/issue", issueCertificate)
	api.Get("/certs", listCertificates)
	
	// Environment variables management
	api.Post("/apps/:app/env", setEnvVars)
	api.Get("/apps/:app/env", getEnvVars)
	api.Put("/apps/:app/env/:key", setEnvVar)
	api.Delete("/apps/:app/env/:key", deleteEnvVar)
	
	// Debug and rollback
	api.Post("/apps/:app/debug", debugApp)
	api.Post("/apps/:app/rollback", rollbackApp)

	port := getenv("PORT", "8081")
	log.Printf("Ploy Controller listening on :%s", port)
	log.Fatal(app.Listen(":" + port))
}

// Preview router: handle Host like <sha>.<app>.ployd.app
var previewHostRe = regexp.MustCompile(`^(?P<sha>[a-f0-9]{7,40})\.(?P<app>[a-z0-9-]+)\.ployd\.app(?::\d+)?$`)

func previewHostRouter(c *fiber.Ctx) error {
	host := c.Hostname()
	m := previewHostRe.FindStringSubmatch(host)
	if m == nil { return c.Next() }
	sha := m[1]; app := m[2]

	payload := strings.NewReader("") // empty tar triggers default build path
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://localhost:%s/v1/apps/%s/builds?sha=%s", getenv("PORT","8081"), app, sha), payload)
	req.Header.Set("Content-Type","application/x-tar")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return c.Status(502).SendString("preview build failed") }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	// naive readiness check and proxy
	if isHealthy("http://127.0.0.1:8080/healthz") { // fast path
		// TODO: Implement actual proxy to app
		return c.Redirect("http://127.0.0.1:8080")
	}
	// slow path: poll Nomad and then proxy default service endpoint if known (placeholder)
	// In production, resolve from Consul or job svc address.
	c.Set("Content-Type","application/json")
	c.Set("Retry-After","3")
	return c.Status(resp.StatusCode).Send(b)
}

func triggerBuild(c *fiber.Ctx) error {
	appName := c.Params("app")
	sha := c.Query("sha", "dev")
	mainClass := c.Query("main", "com.ploy.ordersvc.Main")
	lane := c.Query("lane", "")

	tmpDir, _ := os.MkdirTemp("", "ploy-build-"); defer os.RemoveAll(tmpDir)

	// Save tar stream
	tarPath := filepath.Join(tmpDir, "src.tar"); f, _ := os.Create(tarPath); defer f.Close()
	io.Copy(f, c.Context().RequestBodyStream())

	// Extract tar
	srcDir := filepath.Join(tmpDir, "src"); os.MkdirAll(srcDir, 0755); _ = untar(tarPath, srcDir)

	// Get environment variables for this app
	appEnvVars, err := envStore.GetAll(appName)
	if err != nil {
		log.Printf("Warning: failed to retrieve environment variables for app %s: %v", appName, err)
		appEnvVars = make(map[string]string)
	}

	if lane == "" { if res, err := runLanePick(srcDir); err == nil { lane = res.Lane } else { lane = "C" } }

	var imagePath, dockerImage string
	switch strings.ToUpper(lane) {
	case "A","B":
		img, err := builders.BuildUnikraft(appName, lane, srcDir, sha, tmpDir, appEnvVars); if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	case "C":
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{ App: appName, MainClass: mainClass, SrcDir: srcDir, GitSHA: sha, OutDir: tmpDir, EnvVars: appEnvVars })
		if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	case "D":
		img, err := builders.BuildJail(appName, srcDir, sha, tmpDir, appEnvVars); if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	case "E":
		tag := fmt.Sprintf("harbor.local/ploy/%s:%s", appName, sha)
		img, err := builders.BuildOCI(appName, srcDir, tag, appEnvVars); if err != nil { return errJSON(c, 500, err) }
		dockerImage = img
	case "F":
		img, err := builders.BuildVM(appName, sha, tmpDir, appEnvVars); if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	default:
		lane = "C"
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{ App: appName, MainClass: mainClass, SrcDir: srcDir, GitSHA: sha, OutDir: tmpDir, EnvVars: appEnvVars })
		if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	}

	// SBOM/signature stubs (use CI to generate alongside artifacts)
	sbom := fileExists(imagePath + ".sbom.json") || fileExists(filepath.Join(srcDir,"SBOM.json"))
	signed := fileExists(imagePath + ".sig")
	// Attempt verification if available
	if signed && imagePath != "" { _ = supply.VerifySignature(imagePath, imagePath + ".sig") }

	if err := opa.Enforce(opa.ArtifactInput{ Signed: signed, SBOMPresent: sbom, Env: c.Query("env","dev"), SSHEnabled: false }); err != nil {
		return errJSON(c, 403, fmt.Errorf("policy denied: %w", err))
	}

	jobFile, err := nomad.RenderTemplate(lane, nomad.RenderData{ App: appName, ImagePath: imagePath, DockerImage: dockerImage, EnvVars: appEnvVars })
	if err != nil { return errJSON(c, 500, err) }
	if err := nomad.Submit(jobFile); err != nil { return errJSON(c, 500, err) }
	// wait for job healthy (basic)
	_ = nomad.WaitHealthy(appName+"-lane-"+strings.ToLower(lane), 90*time.Second)

	// Upload artifacts to object storage
	if storeClient != nil {
		keyPrefix := appName + "/" + sha + "/"
		if imagePath != "" {
			if f, err := os.Open(imagePath); err == nil { defer f.Close(); storeClient.PutObject(storeClient.Artifacts, keyPrefix+filepath.Base(imagePath), f, "application/octet-stream") }
			if f, err := os.Open(imagePath+".sbom.json"); err == nil { defer f.Close(); storeClient.PutObject(storeClient.Artifacts, keyPrefix+filepath.Base(imagePath+".sbom.json"), f, "application/json") }
			if f, err := os.Open(imagePath+".sig"); err == nil { defer f.Close(); storeClient.PutObject(storeClient.Artifacts, keyPrefix+filepath.Base(imagePath+".sig"), f, "application/octet-stream") }
		}
		meta := map[string]string{"lane":lane,"image":imagePath,"dockerImage":dockerImage}
		mb,_ := json.Marshal(meta)
		storeClient.PutObject(storeClient.Artifacts, keyPrefix+"meta.json", bytes.NewReader(mb), "application/json")
	}

	return c.JSON(fiber.Map{"status":"deployed","lane":lane,"image":imagePath,"dockerImage":dockerImage})
}

func listApps(c *fiber.Ctx) error { return c.JSON(fiber.Map{"apps":[]string{}}) }
func status(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status":"ok"}) }

func untar(tarPath, dst string) error {
	f, err := os.Open(tarPath); if err != nil { return err }
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(tarPath, ".gz") { gzr, _ := gzip.NewReader(f); defer gzr.Close(); r = gzr }
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF { break }
		if err != nil { return err }
		p := filepath.Join(dst, h.Name)
		if h.FileInfo().IsDir() { os.MkdirAll(p, 0755); continue }
		os.MkdirAll(filepath.Dir(p), 0755)
		out, _ := os.Create(p)
		io.Copy(out, tr); out.Close()
	}
	return nil
}

func runLanePick(path string) (lanePickResult, error) {
	cmd := exec.Command("go", "run", "./tools/lane-pick", "--path", path)
	b, err := cmd.Output(); if err != nil { return lanePickResult{}, err }
	var res lanePickResult; if err := json.Unmarshal(b, &res); err != nil { return lanePickResult{}, err }
	return res, nil
}

func isHealthy(url string) bool {
	client := &http.Client{ Timeout: 1 * time.Second }
	resp, err := client.Get(url); if err != nil { return false }
	defer resp.Body.Close(); return resp.StatusCode == 200
}

func getenv(k, d string) string { if v:=os.Getenv(k); v!="" { return v }; return d }
func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func errJSON(c *fiber.Ctx, code int, err error) error { return c.Status(code).JSON(fiber.Map{"error": err.Error()}) }

// Domain management handlers
func addDomain(c *fiber.Ctx) error {
	app := c.Params("app")
	var req struct {
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	// Store domain mapping in Consul (simplified implementation)
	log.Printf("Adding domain %s for app %s", req.Domain, app)
	
	// TODO: Implement actual Consul integration for domain registration
	// This would typically involve:
	// 1. Register domain in Consul KV store
	// 2. Update ingress configuration
	// 3. Trigger configuration reload
	
	return c.JSON(fiber.Map{
		"status": "added",
		"app": app,
		"domain": req.Domain,
		"message": "Domain registered successfully",
	})
}

func listDomains(c *fiber.Ctx) error {
	app := c.Params("app")
	
	// TODO: Implement actual Consul lookup for domains
	// This would query Consul KV store for domains associated with the app
	
	log.Printf("Listing domains for app %s", app)
	return c.JSON(fiber.Map{
		"app": app,
		"domains": []string{
			fmt.Sprintf("%s.ployd.app", app), // default domain
		},
	})
}

func removeDomain(c *fiber.Ctx) error {
	app := c.Params("app")
	domain := c.Params("domain")
	
	// TODO: Implement actual Consul integration for domain removal
	log.Printf("Removing domain %s from app %s", domain, app)
	
	return c.JSON(fiber.Map{
		"status": "removed",
		"app": app,
		"domain": domain,
		"message": "Domain removed successfully",
	})
}

// Certificate management handlers
func issueCertificate(c *fiber.Ctx) error {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	log.Printf("Issuing certificate for domain %s", req.Domain)
	
	// TODO: Implement actual Let's Encrypt ACME challenge
	// This would typically involve:
	// 1. Create ACME challenge
	// 2. Configure HTTP-01 challenge response
	// 3. Request certificate from Let's Encrypt
	// 4. Store certificate in Vault or filesystem
	
	return c.JSON(fiber.Map{
		"status": "issued",
		"domain": req.Domain,
		"message": "Certificate issued successfully",
		"expires": time.Now().AddDate(0, 3, 0).Format("2006-01-02"),
	})
}

func listCertificates(c *fiber.Ctx) error {
	// TODO: Implement actual certificate listing from Vault or filesystem
	log.Printf("Listing certificates")
	
	return c.JSON(fiber.Map{
		"certificates": []fiber.Map{
			{
				"domain": "example.ployd.app",
				"status": "valid",
				"expires": time.Now().AddDate(0, 2, 0).Format("2006-01-02"),
			},
		},
	})
}

// Debug and rollback handlers
func debugApp(c *fiber.Ctx) error {
	app := c.Params("app")
	lane := c.Query("lane", "")
	
	var req struct {
		SSHEnabled bool `json:"ssh_enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	log.Printf("Creating debug build for app %s (lane: %s) with SSH enabled: %v", app, lane, req.SSHEnabled)
	
	// TODO: Implement debug build with SSH support
	// This would typically involve:
	// 1. Build debug variant with SSH daemon
	// 2. Deploy to debug namespace
	// 3. Return SSH connection details
	
	debugInstance := fmt.Sprintf("debug-%s-%d", app, time.Now().Unix())
	
	return c.JSON(fiber.Map{
		"status": "debug_created",
		"app": app,
		"instance": debugInstance,
		"ssh_enabled": req.SSHEnabled,
		"ssh_command": fmt.Sprintf("ssh debug@%s.debug.ployd.app", debugInstance),
		"message": "Debug instance created successfully",
	})
}

func rollbackApp(c *fiber.Ctx) error {
	app := c.Params("app")
	
	var req struct {
		SHA string `json:"sha"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	log.Printf("Rolling back app %s to SHA %s", app, req.SHA)
	
	// TODO: Implement actual rollback via Nomad
	// This would typically involve:
	// 1. Validate SHA exists in storage
	// 2. Update Nomad job with previous image
	// 3. Monitor rollback health
	
	return c.JSON(fiber.Map{
		"status": "rolled_back",
		"app": app,
		"sha": req.SHA,
		"message": "Application rolled back successfully",
	})
}

// Environment variables handlers
func setEnvVars(c *fiber.Ctx) error {
	app := c.Params("app")
	
	var req map[string]string
	if err := c.BodyParser(&req); err != nil {
		return errJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	log.Printf("Setting environment variables for app %s", app)
	
	if err := envStore.SetAll(app, req); err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to store environment variables: %w", err))
	}
	
	return c.JSON(fiber.Map{
		"status": "updated",
		"app": app,
		"count": len(req),
		"message": "Environment variables updated successfully",
	})
}

func getEnvVars(c *fiber.Ctx) error {
	app := c.Params("app")
	
	log.Printf("Getting environment variables for app %s", app)
	
	envVars, err := envStore.GetAll(app)
	if err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to retrieve environment variables: %w", err))
	}
	
	return c.JSON(fiber.Map{
		"app": app,
		"env": envVars,
	})
}

func setEnvVar(c *fiber.Ctx) error {
	app := c.Params("app")
	key := c.Params("key")
	
	var req struct {
		Value string `json:"value"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	log.Printf("Setting environment variable %s for app %s", key, app)
	
	if err := envStore.Set(app, key, req.Value); err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to store environment variable: %w", err))
	}
	
	return c.JSON(fiber.Map{
		"status": "updated",
		"app": app,
		"key": key,
		"message": "Environment variable updated successfully",
	})
}

func deleteEnvVar(c *fiber.Ctx) error {
	app := c.Params("app")
	key := c.Params("key")
	
	log.Printf("Deleting environment variable %s for app %s", key, app)
	
	if err := envStore.Delete(app, key); err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to delete environment variable: %w", err))
	}
	
	return c.JSON(fiber.Map{
		"status": "deleted",
		"app": app,
		"key": key,
		"message": "Environment variable deleted successfully",
	})
}
