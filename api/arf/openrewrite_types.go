package arf

// OpenRewriteRecipeRequest represents a request to execute an OpenRewrite recipe
type OpenRewriteRecipeRequest struct {
	RecipeClass      string `json:"recipe_class"`
	RecipeGroup      string `json:"recipe_group"`
	RecipeArtifact   string `json:"recipe_artifact"`
	RecipeVersion    string `json:"recipe_version"`
	RepoPath         string `json:"repo_path"`
	JobID            string `json:"job_id"`            // Nomad job ID (will be set after job submission)
	TransformationID string `json:"transformation_id"` // UUID from ARF handler
}
