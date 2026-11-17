package api

// JavaVersionsResponse contains observed dependency sets, without compatibility claims.
type JavaVersionsResponse struct {
	Versions [][]string `json:"versions"`
}
