package arf

// RecipeFilter contains filtering options for recipe listing
type RecipeFilter struct {
	Language  string
	Category  string
	Tags      []string
	Author    string
	Pack      string
	Version   string
	Limit     int
	Offset    int
	MinRating float64
	SortBy    string
	SortOrder string
}

// CommandFlags contains common flags for recipe commands
type CommandFlags struct {
	DryRun       bool
	Force        bool
	Verbose      bool
	Strict       bool
	OutputFormat string
	OutputFile   string
	Name         string
	Template     string
	Interactive  bool
}

// catalogRecipe represents a lightweight recipe from catalog endpoints
type catalogRecipe struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Pack        string   `json:"pack"`
	Version     string   `json:"version"`
}

// UploadFlags contains flags for the upload command (legacy)
type UploadFlags struct {
	DryRun bool
	Force  bool
	Name   string
}
