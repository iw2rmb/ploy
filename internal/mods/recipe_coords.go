package mods

import "fmt"

// validateRecipeCoords ensures group, artifact, version are present.
func validateRecipeCoords(group, artifact, version, stepID, recipe string) error {
	if group == "" || artifact == "" || version == "" {
		if stepID == "" {
			return fmt.Errorf("missing recipe coordinates (coords.group/artifact/version)")
		}
		if recipe == "" {
			recipe = "<unnamed>"
		}
		return fmt.Errorf("missing recipe coordinates: please set coords.group/artifact/version for recipe %q in step %s", recipe, stepID)
	}
	return nil
}
