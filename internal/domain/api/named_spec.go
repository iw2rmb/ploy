package api

// NamedSpecCatalogEntry identifies a named spec discovered from a Git repository.
type NamedSpecCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
}

// NamedSpecListResponse is returned by GET /v1/specs.
type NamedSpecListResponse struct {
	Specs []NamedSpecCatalogEntry `json:"specs"`
}
