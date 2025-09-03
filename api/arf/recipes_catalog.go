package arf

import (
    "encoding/json"
    "fmt"
    "sort"
    "strings"

    "github.com/gofiber/fiber/v2"
)

// RecipeMeta describes a single OpenRewrite recipe entry in the catalog.
type RecipeMeta struct {
    ID          string   `json:"id"`
    DisplayName string   `json:"display_name"`
    Description string   `json:"description"`
    Tags        []string `json:"tags"`
    Pack        string   `json:"pack"`
    Version     string   `json:"version"`
}

// RecipesCatalog provides listing/searching recipes.
type RecipesCatalog struct {
    items map[string]RecipeMeta
}

func NewRecipesCatalog() *RecipesCatalog {
    return &RecipesCatalog{items: make(map[string]RecipeMeta)}
}

// BuildFromYAMLs builds the catalog from in-memory YAML blobs discovered in recipe packs.
// For Phase 1, we support a minimal subset of the OpenRewrite recipe descriptor.
func (c *RecipesCatalog) BuildFromYAMLs(yamls [][]byte, pack, version string) error {
    for _, y := range yamls {
        id := parseYAMLField(string(y), "name:")
        if id == "" {
            continue
        }
        meta := RecipeMeta{
            ID:          strings.TrimSpace(id),
            DisplayName: strings.TrimSpace(parseYAMLField(string(y), "displayName:")),
            Description: strings.TrimSpace(parseYAMLField(string(y), "description:")),
            Tags:        parseYAMLList(string(y), "tags:"),
            Pack:        pack,
            Version:     version,
        }
        c.items[meta.ID] = meta
    }
    return nil
}

// List returns a sorted slice of recipes.
func (c *RecipesCatalog) List(pack, version string, limit int) []RecipeMeta {
    out := make([]RecipeMeta, 0, len(c.items))
    for _, v := range c.items {
        if pack != "" && v.Pack != pack {
            continue
        }
        if version != "" && v.Version != version {
            continue
        }
        out = append(out, v)
    }
    sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
    if limit > 0 && len(out) > limit {
        out = out[:limit]
    }
    return out
}

// Search performs a case-insensitive search over ID, DisplayName, and Description.
func (c *RecipesCatalog) Search(query string, limit int) []RecipeMeta {
    q := strings.ToLower(strings.TrimSpace(query))
    if q == "" {
        return c.List("", "", limit)
    }
    out := make([]RecipeMeta, 0)
    for _, v := range c.items {
        text := strings.ToLower(v.ID + " " + v.DisplayName + " " + v.Description)
        if strings.Contains(text, q) {
            out = append(out, v)
        }
    }
    sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
    if limit > 0 && len(out) > limit {
        out = out[:limit]
    }
    return out
}

// GetByID returns a recipe by fully-qualified ID.
func (c *RecipesCatalog) GetByID(id string) *RecipeMeta {
    v, ok := c.items[id]
    if !ok {
        return nil
    }
    return &v
}

// Serialize returns a JSON representation of the catalog (for persistence later).
func (c *RecipesCatalog) Serialize() ([]byte, error) {
    list := c.List("", "", 0)
    return json.Marshal(list)
}

// parseYAMLField extracts a single-line scalar from a minimal YAML string.
func parseYAMLField(src, key string) string {
    lines := strings.Split(src, "\n")
    for _, ln := range lines {
        ln = strings.TrimSpace(ln)
        if strings.HasPrefix(ln, key) {
            return strings.TrimSpace(strings.TrimPrefix(ln, key))
        }
    }
    return ""
}

// parseYAMLList extracts a simple single-level list following a key.
func parseYAMLList(src, key string) []string {
    lines := strings.Split(src, "\n")
    var out []string
    keyFound := false
    for _, ln := range lines {
        if !keyFound {
            if strings.HasPrefix(strings.TrimSpace(ln), key) {
                keyFound = true
            }
            continue
        }
        s := strings.TrimSpace(ln)
        if strings.HasPrefix(s, "-") {
            out = append(out, strings.TrimSpace(strings.TrimPrefix(s, "-")))
            continue
        }
        // end of list on first non-item line
        break
    }
    return out
}

// --- HTTP Handlers ---

type RecipesHandler struct {
    catalog *RecipesCatalog
    indexer *RecipesIndexer
}

func NewRecipesHandler(cat *RecipesCatalog) *RecipesHandler {
    return &RecipesHandler{catalog: cat}
}

// SetIndexer injects an indexer instance for refresh operations (used in Phase 1).
func (h *RecipesHandler) SetIndexer(idx *RecipesIndexer) { h.indexer = idx }

// ListRecipes handles GET /v1/arf/recipes
// Query params: query, pack, version, limit
func (h *RecipesHandler) ListRecipes(c *fiber.Ctx) error {
    q := c.Query("query")
    pack := c.Query("pack")
    version := c.Query("version")
    limit := atoiOrZero(c.Query("limit"))

    var list []RecipeMeta
    if strings.TrimSpace(q) != "" {
        list = h.catalog.Search(q, limit)
    } else {
        list = h.catalog.List(pack, version, limit)
    }
    return c.JSON(list)
}

// GetRecipe handles GET /v1/arf/recipes/:id
func (h *RecipesHandler) GetRecipe(c *fiber.Ctx) error {
    id := c.Params("id")
    if id == "" {
        return c.Status(400).JSON(map[string]string{"error": "id is required"})
    }
    r := h.catalog.GetByID(id)
    if r == nil {
        return c.Status(404).JSON(map[string]string{"error": fmt.Sprintf("recipe %s not found", id)})
    }
    return c.JSON(r)
}

// RefreshRecipes handles POST /v1/arf/recipes/refresh
// Body optional JSON: {"packs":[{"name":"rewrite-java","version":"2.20.0"}, ...]}
func (h *RecipesHandler) RefreshRecipes(c *fiber.Ctx) error {
    if h.indexer == nil {
        return c.Status(500).JSON(map[string]string{"error": "indexer not configured"})
    }
    var req struct{
        Packs []PackSpec `json:"packs"`
    }
    if err := c.BodyParser(&req); err != nil {
        // Ignore body errors; proceed with defaults
        req.Packs = nil
    }
    if len(req.Packs) == 0 {
        req.Packs = []PackSpec{{Name: "rewrite-java", Version: "2.20.0"}}
    }
    cat, err := h.indexer.Refresh(c.Context(), req.Packs)
    if err != nil {
        return c.Status(500).JSON(map[string]string{"error": err.Error()})
    }
    h.catalog = cat
    return c.JSON(map[string]interface{}{"status": "ok", "count": len(cat.List("", "", 0))})
}

// Note: Handlers use fiber.Ctx directly for compatibility with Fiber routing and tests.

func atoiOrZero(s string) int {
    for _, ch := range s {
        if ch < '0' || ch > '9' {
            return 0
        }
    }
    if s == "" {
        return 0
    }
    var n int
    for i := 0; i < len(s); i++ {
        n = n*10 + int(s[i]-'0')
    }
    return n
}
