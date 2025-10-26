// registry_types.go groups helper structs shared by the registry handlers.
package httpserver

type registryUploadRequest struct {
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
	NodeID    string `json:"node_id"`
}

type registryUploadProgressRequest struct {
	Size int64 `json:"size"`
}

type registryCommitRequest struct {
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
}

type ociManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
}

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}
