package mods

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
		entries = append(entries, recipeEntry(name, "", "", ""))
	}
	return entries
}
