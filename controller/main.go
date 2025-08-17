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
	"github.com/ploy/ploy/internal/storage"
)

type lanePickResult struct {
	Lane string `json:"lane"`
	Language string `json:"language"`
	Reasons []string `json:"reasons"`
}

var storeClient *storage.Client

func main(){
	app := fiber.New()
	app.Use(previewHostRouter)

	cfgPath := getenv("PLOY_STORAGE_CONFIG", "configs/storage-config.yaml")
	if rootCfg, err := config.Load(cfgPath); err == nil {
		if c, err := storage.New(rootCfg.Storage); err == nil { storeClient = c }
	}

	api := app.Group("/v1")
	api.Post("/apps/:app/builds", triggerBuild)
	api.Get("/apps", listApps)
	api.Get("/status/:app", status)

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
		return c.Proxy("http://127.0.0.1:8080")
	}
	// slow path: poll Nomad and then proxy default service endpoint if known (placeholder)
	// In production, resolve from Consul or job svc address.
		return c.Proxy("http://127.0.0.1:8080")
	}
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

	if lane == "" { if res, err := runLanePick(srcDir); err == nil { lane = res.Lane } else { lane = "C" } }

	var imagePath, dockerImage string
	switch strings.ToUpper(lane) {
	case "A","B":
		img, err := builders.BuildUnikraft(appName, lane, srcDir, sha, tmpDir); if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	case "C":
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{ App: appName, MainClass: mainClass, SrcDir: srcDir, GitSHA: sha, OutDir: tmpDir })
		if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	case "D":
		img, err := builders.BuildJail(appName, srcDir, sha, tmpDir); if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	case "E":
		tag := fmt.Sprintf("harbor.local/ploy/%s:%s", appName, sha)
		img, err := builders.BuildOCI(appName, srcDir, tag); if err != nil { return errJSON(c, 500, err) }
		dockerImage = img
	case "F":
		img, err := builders.BuildVM(appName, sha, tmpDir); if err != nil { return errJSON(c, 500, err) }
		imagePath = img
	default:
		lane = "C"
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{ App: appName, MainClass: mainClass, SrcDir: srcDir, GitSHA: sha, OutDir: tmpDir })
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

	jobFile, err := nomad.RenderTemplate(lane, nomad.RenderData{ App: appName, ImagePath: imagePath, DockerImage: dockerImage })
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
