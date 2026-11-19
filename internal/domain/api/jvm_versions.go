package api

// JVMVersionsResponse contains observed dependency sets, without compatibility claims.
type JVMVersionsResponse struct {
	Versions [][]string `json:"versions"`
}
