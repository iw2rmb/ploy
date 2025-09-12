package transflow

import "fmt"

// validateRecipeCoords ensures group, artifact, version are present.
func validateRecipeCoords(group, artifact, version, stepID string) error {
    if group == "" || artifact == "" || version == "" {
        if stepID == "" {
            return fmt.Errorf("missing recipe coordinates (recipe_group/artifact/version)")
        }
        return fmt.Errorf("missing recipe coordinates: please set recipe_group, recipe_artifact, recipe_version in step %s", stepID)
    }
    return nil
}

