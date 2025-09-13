package mods

import (
	"os"
	"path/filepath"
	"strings"
)

// preSubstituteRecipe fills in RECIPE_CLASS/RECIPE_COORDS/RECIPE_TIMEOUT in the rendered HCL
// and writes a .pre.hcl file next to the input template. Does not mutate process env.
func preSubstituteRecipe(renderedHCLPath, recipeClass, recipeCoords, recipeTimeout string) (string, error) {
	if recipeClass == "" {
		recipeClass = "org.openrewrite.java.migrate.UpgradeToJava17"
	}
	if recipeTimeout == "" {
		recipeTimeout = "10m"
	}
	hb, err := os.ReadFile(renderedHCLPath)
	if err != nil {
		return "", err
	}
	pre := strings.NewReplacer(
		"${RECIPE_CLASS}", recipeClass,
		"${RECIPE_COORDS}", recipeCoords,
		"${RECIPE_TIMEOUT}", recipeTimeout,
	).Replace(string(hb))
	prePath := strings.ReplaceAll(renderedHCLPath, ".rendered.hcl", ".pre.hcl")
	if prePath == renderedHCLPath {
		// Fallback: ensure new suffix
		dir := filepath.Dir(renderedHCLPath)
		base := filepath.Base(renderedHCLPath)
		prePath = filepath.Join(dir, strings.TrimSuffix(base, ".hcl")+".pre.hcl")
	}
	if err := os.WriteFile(prePath, []byte(pre), 0644); err != nil {
		return "", err
	}
	return prePath, nil
}
