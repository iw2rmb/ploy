package mods

import "strings"

func recipeEntry(name, group, artifact, version string) RecipeEntry {
	return RecipeEntry{
		Name: name,
		Coords: RecipeCoordinates{
			Group:    group,
			Artifact: artifact,
			Version:  version,
		},
	}
}

func recipeNames(names ...string) []RecipeEntry {
	entries := make([]RecipeEntry, 0, len(names))
	for _, name := range names {
		artifact := strings.NewReplacer(" ", "-", ".", "-").Replace(strings.ToLower(name))
		entries = append(entries, recipeEntry(name, "org.example", artifact, "1.0.0"))
	}
	return entries
}
