package catalog

// CatalogEntry models a recipe entry in the persisted catalog snapshot.
// This consolidates the schema in one place for internal use.
type CatalogEntry struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Pack        string   `json:"pack"`
	Version     string   `json:"version"`
}
