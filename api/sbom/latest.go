package sbom

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

func hashRepo(repo string) string {
	h := sha1.Sum([]byte(repo))
	return hex.EncodeToString(h[:])
}

func latestPointerKey(repo string) string {
	return fmt.Sprintf("mods/sbom/latest/%s.json", hashRepo(repo))
}

// GetLatest returns pointer metadata to the latest SBOM for a given repo (by URL)
func (h *Handler) GetLatest(c *fiber.Ctx) error {
	repo := c.Query("repo")
	if repo == "" || h.storage == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "repo required"})
	}
	key := latestPointerKey(repo)
	r, err := h.storage.Get(c.Context(), key)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "latest SBOM not found"})
	}
	defer func() { _ = r.Close() }()
	var data map[string]interface{}
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to parse pointer"})
	}
	// Ensure we return a stable envelope
	if data["updated_at"] == nil {
		data["updated_at"] = time.Now().UTC()
	}
	return c.JSON(data)
}
