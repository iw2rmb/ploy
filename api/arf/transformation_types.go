package arf

// TransformRequest represents a transformation request
type TransformRequest struct {
	RecipeID string   `json:"recipe_id" validate:"required"`
	Type     string   `json:"type,omitempty"` // "openrewrite" or empty for regular recipes
	Codebase Codebase `json:"codebase" validate:"required"`
}

// NotFoundError represents a resource not found error
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}
