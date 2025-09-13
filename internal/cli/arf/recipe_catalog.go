package arf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// listCatalogRecipes lists recipes from the lightweight catalog endpoints
func listCatalogRecipes(outputFormat string) error {
	url := fmt.Sprintf("%s/arf/recipes", arfControllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to retrieve catalog: %w", err)
	}
	items, err := parseCatalogList(response)
	if err != nil {
		return err
	}
	return printCatalog(items, outputFormat, false)
}

// searchCatalogRecipes searches recipes in catalog mode
func searchCatalogRecipes(query, outputFormat string, verbose bool) error {
	url := fmt.Sprintf("%s/arf/recipes?query=%s", arfControllerURL, query)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to search catalog: %w", err)
	}
	items, err := parseCatalogList(response)
	if err != nil {
		return err
	}
	return printCatalog(items, outputFormat, verbose)
}

// parseCatalogList parses the catalog array payload (used in tests)
func parseCatalogList(data []byte) ([]catalogRecipe, error) {
	var items []catalogRecipe
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse catalog list: %w", err)
	}
	return items, nil
}

// printCatalog formats and prints catalog recipe data
func printCatalog(items []catalogRecipe, format string, verbose bool) error {
	switch format {
	case "json":
		out, _ := json.MarshalIndent(items, "", "  ")
		fmt.Println(string(out))
		return nil
	case "yaml":
		// minimal YAML via json2yaml-style is not available; fallback to json for now
		out, _ := json.MarshalIndent(items, "", "  ")
		fmt.Println(string(out))
		return nil
	default:
		if len(items) == 0 {
			fmt.Println("No recipes found")
			return nil
		}
		// simple table-like output
		fmt.Printf("ID\tPACK\tVERSION\tNAME\n")
		for _, it := range items {
			name := it.DisplayName
			if name == "" {
				name = it.ID
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", it.ID, it.Pack, it.Version, name)
			if verbose && it.Description != "" {
				fmt.Printf("  %s\n", it.Description)
			}
		}
		fmt.Printf("Total: %d recipes\n", len(items))
		return nil
	}
}

// getCatalogSuggestions fetches catalog and returns top suggestion IDs for a given raw recipeID
func getCatalogSuggestions(rawID string) ([]string, error) {
	// Query by last segment to broaden matches
	seg := rawID
	if dot := strings.LastIndex(rawID, "."); dot != -1 && dot+1 < len(rawID) {
		seg = rawID[dot+1:]
	}
	url := fmt.Sprintf("%s/arf/recipes?query=%s", arfControllerURL, seg)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	items, err := parseCatalogList(response)
	if err != nil {
		return nil, err
	}
	return generateRecipeSuggestions(rawID, items), nil
}

// generateRecipeSuggestions ranks simple suggestions from catalog items
func generateRecipeSuggestions(rawID string, items []catalogRecipe) []string {
	// Prefer exact ID match (should not happen if called on failure), then same pack/name family, then others
	last := rawID
	if dot := strings.LastIndex(rawID, "."); dot != -1 && dot+1 < len(rawID) {
		last = rawID[dot+1:]
	}
	packHint := ""
	if idx := strings.Index(rawID, "."); idx != -1 {
		packHint = rawID[:idx]
	}
	// Score items
	type scored struct {
		id    string
		score int
	}
	scores := make([]scored, 0, len(items))
	for _, it := range items {
		s := 0
		if it.ID == rawID {
			s += 100
		}
		if strings.Contains(it.ID, last) {
			s += 20
		}
		if it.DisplayName != "" && strings.Contains(it.DisplayName, last) {
			s += 10
		}
		if packHint != "" && strings.Contains(it.ID, packHint) {
			s += 5
		}
		if s > 0 {
			scores = append(scores, scored{id: it.ID, score: s})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	out := []string{}
	seen := map[string]bool{}
	for _, sc := range scores {
		if !seen[sc.id] {
			out = append(out, sc.id)
			seen[sc.id] = true
			if len(out) >= 5 {
				break
			}
		}
	}
	return out
}
