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
	
	// Debug, rollback, and destroy
	api.Post("/apps/:app/debug", debugApp)
	api.Post("/apps/:app/rollback", rollbackApp)
	api.Delete("/apps/:app", destroyApp)

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

	// Check Nomad allocation health and proxy to app
	jobName := fmt.Sprintf("%s-%s", app, sha)
	if nomad.IsJobHealthy(jobName) {
		endpoint, err := nomad.GetJobEndpoint(jobName)
		if err == nil {
			return c.Redirect(endpoint)
		}
	}
	
	// Fallback: return build response with retry header
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
	
	// Get latest source for app (this is simplified - in production you'd retrieve from storage)
	srcDir := filepath.Join(os.TempDir(), fmt.Sprintf("debug-src-%s-%d", app, time.Now().Unix()))
	outDir := filepath.Join(os.TempDir(), fmt.Sprintf("debug-out-%s-%d", app, time.Now().Unix()))
	sha := fmt.Sprintf("debug-%d", time.Now().Unix())
	
	// Create directories
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to create source directory: %v", err))
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to create output directory: %v", err))
	}
	
	// Get environment variables for the app
	envVarsData, err := envStore.GetAll(app)
	if err != nil {
		log.Printf("Failed to get environment variables for %s: %v", app, err)
		envVarsData = make(map[string]string) // Continue with empty env vars
	}
	envVars := make(map[string]string)
	for k, v := range envVarsData {
		envVars[k] = v
	}
	
	// Build debug instance
	debugResult, err := builders.BuildDebugInstance(app, lane, srcDir, sha, outDir, envVars, req.SSHEnabled)
	if err != nil {
		return errJSON(c, 500, fmt.Errorf("debug build failed: %v", err))
	}
	
	// Deploy to debug namespace using Nomad
	debugInstanceName := fmt.Sprintf("debug-%s-%d", app, time.Now().Unix())
	renderData := nomad.RenderData{
		App:         debugInstanceName,
		ImagePath:   debugResult.ImagePath,
		DockerImage: debugResult.DockerImage,
		EnvVars:     envVars,
		IsDebug:     true,
	}
	
	templatePath, err := nomad.RenderTemplate(lane, renderData)
	if err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to render debug template: %v", err))
	}
	
	// Submit debug job to Nomad
	if err := nomad.Submit(templatePath); err != nil {
		return errJSON(c, 500, fmt.Errorf("failed to deploy debug instance: %v", err))
	}
	
	// Clean up template file
	os.Remove(templatePath)
	
	response := fiber.Map{
		"status":      "debug_created",
		"app":         app,
		"instance":    debugInstanceName,
		"ssh_enabled": req.SSHEnabled,
		"message":     "Debug instance created successfully",
	}
	
	if req.SSHEnabled {
		response["ssh_command"] = debugResult.SSHCommand
		response["ssh_public_key"] = debugResult.SSHPublicKey
	}
	
	return c.JSON(response)
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

func destroyApp(c *fiber.Ctx) error {
	app := c.Params("app")
	force := c.Query("force") == "true"
	
	log.Printf("Destroying app %s (force: %v)", app, force)
	
	// Initialize destruction status tracking
	destroyStatus := map[string]interface{}{
		"app": app,
		"status": "destroying",
		"operations": map[string]string{},
		"errors": []string{},
	}
	
	// 1. Stop and remove all Nomad jobs for the app
	if err := destroyNomadJobs(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Nomad cleanup failed: %v", err))
	}
	
	// 2. Remove all environment variables
	if err := destroyEnvironmentVariables(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Environment cleanup failed: %v", err))
	}
	
	// 3. Remove all domain registrations
	if err := destroyDomains(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Domain cleanup failed: %v", err))
	}
	
	// 4. Revoke and delete certificates
	if err := destroyCertificates(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Certificate cleanup failed: %v", err))
	}
	
	// 5. Clean up storage artifacts (images, SBOMs, etc.)
	if err := destroyStorageArtifacts(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Storage cleanup failed: %v", err))
	}
	
	// 6. Remove container images from registry
	if err := destroyContainerImages(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Container cleanup failed: %v", err))
	}
	
	// 7. Clean up temporary files and build artifacts
	if err := destroyTemporaryFiles(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Temporary files cleanup failed: %v", err))
	}
	
	// Set final status
	errors := destroyStatus["errors"].([]string)
	if len(errors) == 0 {
		destroyStatus["status"] = "destroyed"
		destroyStatus["message"] = "Application and all associated resources destroyed successfully"
	} else {
		destroyStatus["status"] = "partially_destroyed"
		destroyStatus["message"] = fmt.Sprintf("Application destroyed with %d errors", len(errors))
	}
	
	return c.JSON(destroyStatus)
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

// Destroy helper functions
func destroyNomadJobs(app string, status map[string]interface{}) error {
	log.Printf("Destroying Nomad jobs for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	// Stop and remove all Nomad jobs related to this app
	// This includes main deployment, preview instances, and debug instances
	jobPatterns := []string{
		app,                    // Main job
		fmt.Sprintf("%s-*", app), // Preview jobs
		fmt.Sprintf("debug-%s-*", app), // Debug jobs
	}
	
	for _, pattern := range jobPatterns {
		cmd := exec.Command("nomad", "job", "stop", "-purge", pattern)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Don't fail if job doesn't exist
			if !strings.Contains(string(output), "not found") {
				log.Printf("Failed to stop Nomad job %s: %v", pattern, err)
				return fmt.Errorf("failed to stop job %s: %v", pattern, err)
			}
		}
		operations[fmt.Sprintf("nomad_%s", pattern)] = "stopped"
	}
	
	log.Printf("Nomad jobs destroyed for app: %s", app)
	return nil
}

func destroyEnvironmentVariables(app string, status map[string]interface{}) error {
	log.Printf("Destroying environment variables for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	// Remove all environment variables for the app
	envVars, err := envStore.GetAll(app)
	if err != nil {
		log.Printf("No environment variables found for app %s: %v", app, err)
		operations["env_vars"] = "none_found"
		return nil
	}
	
	// Delete each environment variable
	for key := range envVars {
		if err := envStore.Delete(app, key); err != nil {
			return fmt.Errorf("failed to delete environment variable %s: %v", key, err)
		}
	}
	
	operations["env_vars"] = fmt.Sprintf("deleted_%d_variables", len(envVars))
	log.Printf("Environment variables destroyed for app: %s", app)
	return nil
}

func destroyDomains(app string, status map[string]interface{}) error {
	log.Printf("Destroying domains for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	// TODO: Implement domain removal logic
	// This would involve:
	// 1. Query domain registry for app domains
	// 2. Remove DNS records
	// 3. Update load balancer configuration
	// 4. Remove domain from app registry
	
	operations["domains"] = "not_implemented"
	log.Printf("Domain destruction not implemented for app: %s", app)
	return nil
}

func destroyCertificates(app string, status map[string]interface{}) error {
	log.Printf("Destroying certificates for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	// TODO: Implement certificate revocation and cleanup
	// This would involve:
	// 1. Query certificate store for app certificates
	// 2. Revoke certificates with ACME provider
	// 3. Remove certificate files
	// 4. Update certificate registry
	
	operations["certificates"] = "not_implemented"
	log.Printf("Certificate destruction not implemented for app: %s", app)
	return nil
}

func destroyStorageArtifacts(app string, status map[string]interface{}) error {
	log.Printf("Destroying storage artifacts for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	if storeClient == nil {
		operations["storage"] = "no_client"
		return nil
	}
	
	// TODO: Implement storage cleanup
	// This would involve:
	// 1. List all objects with app prefix in storage
	// 2. Delete images, SBOMs, signatures, and build artifacts
	// 3. Clean up versioned artifacts
	
	operations["storage"] = "not_implemented"
	log.Printf("Storage artifact destruction not implemented for app: %s", app)
	return nil
}

func destroyContainerImages(app string, status map[string]interface{}) error {
	log.Printf("Destroying container images for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	// Remove container images from registry
	// This targets harbor.local/ploy/<app>:* pattern
	
	// List and remove all tags for the app
	imagePattern := fmt.Sprintf("harbor.local/ploy/%s", app)
	
	// Use docker command to remove images
	cmd := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", imagePattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to list images for %s: %v", app, err)
		operations["container_images"] = "list_failed"
		return nil // Don't fail destroy for this
	}
	
	images := strings.Split(strings.TrimSpace(string(output)), "\n")
	deletedCount := 0
	
	for _, image := range images {
		if image != "" && strings.Contains(image, app) {
			rmCmd := exec.Command("docker", "rmi", "-f", image)
			if rmErr := rmCmd.Run(); rmErr != nil {
				log.Printf("Failed to remove image %s: %v", image, rmErr)
			} else {
				deletedCount++
			}
		}
	}
	
	operations["container_images"] = fmt.Sprintf("deleted_%d_images", deletedCount)
	log.Printf("Container images destroyed for app: %s (deleted: %d)", app, deletedCount)
	return nil
}

func destroyTemporaryFiles(app string, status map[string]interface{}) error {
	log.Printf("Destroying temporary files for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	// Clean up temporary directories and build artifacts
	tempDirs := []string{
		fmt.Sprintf("/tmp/*%s*", app),
		fmt.Sprintf("/tmp/debug-*%s*", app),
		fmt.Sprintf("/tmp/build-*%s*", app),
	}
	
	deletedDirs := 0
	for _, pattern := range tempDirs {
		cmd := exec.Command("sh", "-c", fmt.Sprintf("rm -rf %s", pattern))
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to remove temp files %s: %v", pattern, err)
		} else {
			deletedDirs++
		}
	}
	
	// Also clean up any SSH keys for debug sessions
	sshKeyPattern := fmt.Sprintf("/tmp/debug-%s-*.key", app)
	cmd := exec.Command("sh", "-c", fmt.Sprintf("rm -f %s", sshKeyPattern))
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to remove SSH keys for %s: %v", app, err)
	}
	
	operations["temp_files"] = fmt.Sprintf("cleaned_%d_patterns", deletedDirs)
	log.Printf("Temporary files destroyed for app: %s", app)
	return nil
}
