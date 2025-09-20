package mods

import (
	"bytes"
	"context"
	crsha1 "crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

// GetArtifacts returns the artifact keys for a given execution
func (h *Handler) GetArtifacts(c *fiber.Ctx) error {
	id := c.Params("id")
	st, err := h.getStatus(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "not_found", "message": "execution not found"}})
	}
	var arts map[string]any
	if st.Result != nil {
		if a, ok := st.Result["artifacts"].(map[string]any); ok {
			arts = a
		}
	}
	if arts == nil {
		arts = map[string]any{}
	}
	return c.JSON(fiber.Map{"artifacts": arts})
}

// DownloadArtifact streams the requested artifact (plan_json|next_json|diff_patch)
func (h *Handler) DownloadArtifact(c *fiber.Ctx) error {
	if h.storage == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": fiber.Map{"code": "storage_disabled", "message": "artifact storage not configured"}})
	}
	id := c.Params("id")
	name := c.Params("name")
	st, err := h.getStatus(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "not_found", "message": "execution not found"}})
	}
	var arts map[string]any
	if st.Result != nil {
		if a, ok := st.Result["artifacts"].(map[string]any); ok {
			arts = a
		}
	}
	if arts == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "no_artifacts", "message": "no artifacts recorded"}})
	}
	keyAny, ok := arts[name]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "artifact_not_found", "message": "artifact not present"}})
	}
	key, _ := keyAny.(string)
	if key == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "artifact_not_found", "message": "artifact not present"}})
	}
	// Validate artifact path safety and prefix
	if !validTransflowArtifactKey(id, key) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fiber.Map{"code": "invalid_artifact_key", "message": "artifact key failed validation"}})
	}
	reader, err := h.storage.Get(c.Context(), key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fiber.Map{"code": "storage_error", "message": err.Error()}})
	}
	defer func() { _ = reader.Close() }()
	// Stream
	c.Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", name))
	if strings.HasSuffix(key, ".json") {
		c.Type("json")
	} else if strings.HasSuffix(key, ".patch") || name == "diff_patch" {
		c.Type("text/plain")
	}
	_, _ = io.Copy(c, reader)
	return nil
}

// UploadArtifact accepts planner/reducer/apply artifacts from remote executors when SeaweedFS is unreachable.
func (h *Handler) UploadArtifact(c *fiber.Ctx) error {
	if h.storage == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": fiber.Map{"code": "storage_disabled", "message": "artifact storage not configured"}})
	}
	id := c.Params("id")
	name := c.Params("name")
	target, ok := artifactUploadTargets[name]
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fiber.Map{"code": "invalid_artifact", "message": "unsupported artifact name"}})
	}
	filename := target.filename
	contentType := target.contentType
	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fiber.Map{"code": "empty_artifact", "message": "artifact payload is empty"}})
	}
	key := fmt.Sprintf("artifacts/mods/%s/%s", id, filename)
	if err := h.storage.Put(c.Context(), key, bytes.NewReader(body), internalStorage.WithContentType(contentType)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fiber.Map{"code": "storage_error", "message": err.Error()}})
	}
	if err := h.recordArtifactKey(id, name, key); err != nil {
		log.Printf("[Mods] warning: failed to record artifact key for %s/%s: %v", id, name, err)
	}
	return c.JSON(fiber.Map{"ok": true, "key": key})
}

var artifactUploadTargets = map[string]struct {
	filename    string
	contentType string
}{
	"plan_json":  {filename: "plan.json", contentType: "application/json"},
	"next_json":  {filename: "next.json", contentType: "application/json"},
	"diff_patch": {filename: "diff.patch", contentType: "text/plain"},
}

func (h *Handler) recordArtifactKey(modID, name, key string) error {
	if h.statusStore == nil {
		return nil
	}
	st, err := h.getStatus(modID)
	if err != nil || st == nil {
		return err
	}
	if st.Result == nil {
		st.Result = map[string]any{}
	}
	arts, _ := st.Result["artifacts"].(map[string]any)
	if arts == nil {
		arts = map[string]any{}
	}
	arts[name] = key
	st.Result["artifacts"] = arts
	return h.storeStatus(*st)
}

// validTransflowArtifactKey enforces prefix and path safety for artifact keys.
func validTransflowArtifactKey(id, key string) bool {
	if id == "" || key == "" {
		return false
	}
	prefix := fmt.Sprintf("artifacts/mods/%s/", id)
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	if strings.Contains(key, "..") {
		return false
	}
	if strings.Contains(key, "\\") {
		return false
	}
	return true
}

// persistArtifacts scans the temp workspace for known Mods artifacts and uploads them to storage.
// Returns a map of artifact logical names to storage keys.
func (h *Handler) persistArtifacts(modID, tempDir string) (map[string]string, error) {
	artifacts := map[string]string{}
	if h.storage == nil {
		return artifacts, nil
	}
	ctx := context.Background()
	// Scan for any SBOMs generated by jobs and persist
	_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".sbom.json") {
			key := fmt.Sprintf("artifacts/mods/%s/%s", modID, info.Name())
			f, _ := os.Open(path)
			defer func() { _ = f.Close() }()
			if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("application/json")); err == nil {
				// Best-effort: normalize a logical name for source/container if detectable
				lname := "sbom"
				n := strings.ToLower(info.Name())
				if strings.Contains(n, "source") || n == ".sbom.json" {
					lname = "source_sbom"
				}
				if strings.Contains(n, "container") {
					lname = "container_sbom"
				}
				artifacts[lname] = key
			}
			// Do not return error to allow other artifacts
			return nil
		}
		return nil
	})
	// Planner plan.json
	planPath := filepath.Join(tempDir, "planner", "out", "plan.json")
	if fi, err := os.Stat(planPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/mods/%s/plan.json", modID)
		f, _ := os.Open(planPath)
		defer func() { _ = f.Close() }()
		if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("application/json")); err == nil {
			artifacts["plan_json"] = key
		}
	}
	// Reducer next.json
	nextPath := filepath.Join(tempDir, "reducer", "out", "next.json")
	if fi, err := os.Stat(nextPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/mods/%s/next.json", modID)
		f, _ := os.Open(nextPath)
		defer func() { _ = f.Close() }()
		if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("application/json")); err == nil {
			artifacts["next_json"] = key
		}
	}
	// ORW diff.patch
	orwApplyOut := filepath.Join(tempDir, "orw-apply", "out")
	diffPath := filepath.Join(orwApplyOut, "diff.patch")
	if fi, err := os.Stat(diffPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/mods/%s/diff.patch", modID)
		f, _ := os.Open(diffPath)
		defer func() { _ = f.Close() }()
		if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("text/plain")); err == nil {
			artifacts["diff_patch"] = key
		}
	}
	// ORW error.log (best-effort)
	errLogPath := filepath.Join(orwApplyOut, "error.log")
	if fi, err := os.Stat(errLogPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/mods/%s/error.log", modID)
		// Contents might be small; stream
		if _, ok := artifacts["error_log"]; !ok {
			f, _ := os.Open(errLogPath)
			defer func() { _ = f.Close() }()
			if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("text/plain")); err == nil {
				artifacts["error_log"] = key
			}
		}
	}
	// Also scan for any top-level diff.patch or error.log
	_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.Name() == "diff.patch" && artifacts["diff_patch"] == "" {
			key := fmt.Sprintf("artifacts/mods/%s/diff.patch", modID)
			f, _ := os.Open(path)
			defer func() { _ = f.Close() }()
			if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("text/plain")); err == nil {
				artifacts["diff_patch"] = key
			}
			return nil
		}
		if info.Name() == "error.log" && artifacts["error_log"] == "" {
			key := fmt.Sprintf("artifacts/mods/%s/error.log", modID)
			f, _ := os.Open(path)
			defer func() { _ = f.Close() }()
			if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("text/plain")); err == nil {
				artifacts["error_log"] = key
			}
			return nil
		}
		return nil
	})
	// If no local diff found or persisted above, check storage proactively for known keys (SeaweedFS-only IO path)
	if _, ok := artifacts["diff_patch"]; !ok {
		keyPrimary := fmt.Sprintf("artifacts/mods/%s/diff.patch", modID)
		keyAlt := fmt.Sprintf("mods/%s/diff.patch", modID)
		if ok, _ := h.storage.Exists(ctx, keyPrimary); ok {
			artifacts["diff_patch"] = keyPrimary
		} else if ok2, _ := h.storage.Exists(ctx, keyAlt); ok2 {
			artifacts["diff_patch"] = keyAlt
		}
	}
	return artifacts, nil
}

// recordLatestSBOM writes a pointer file under mods/sbom/latest/<repo-hash>.json
func (h *Handler) recordLatestSBOM(repo, storageKey, sha, modID string) {
	if h.storage == nil || repo == "" || storageKey == "" {
		return
	}
	sum := crsha1.Sum([]byte(repo))
	slug := hex.EncodeToString(sum[:])
	data := map[string]interface{}{
		"repo":        repo,
		"sha":         sha,
		"storage_key": storageKey,
		"mod_id":      modID,
		"updated_at":  time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.Marshal(data)
	now := time.Now().UTC().Format(time.RFC3339)
	latestKey := fmt.Sprintf("mods/sbom/latest/%s.json", slug)
	_ = h.storage.Put(context.Background(), latestKey, bytes.NewReader(b), internalStorage.WithContentType("application/json"))
	// Also append history entry for discoverability
	histKey := fmt.Sprintf("mods/sbom/history/%s/%s.json", slug, now)
	_ = h.storage.Put(context.Background(), histKey, bytes.NewReader(b), internalStorage.WithContentType("application/json"))
}
