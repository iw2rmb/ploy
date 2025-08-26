package seaweedfs

// AssignResponse represents the response from SeaweedFS volume assignment
type AssignResponse struct {
	FID       string `json:"fid"`        // File ID for the assigned volume
	URL       string `json:"url"`        // Volume server URL
	PublicURL string `json:"publicUrl"`  // Public URL if configured
	Count     int    `json:"count"`      // Number of file ids assigned
	Error     string `json:"error,omitempty"` // Error message if any
}

// LookupResponse represents the response from SeaweedFS volume lookup
type LookupResponse struct {
	VolumeID  string     `json:"volumeId"`
	Locations []Location `json:"locations"`
	Error     string     `json:"error,omitempty"`
}

// Location represents a volume server location
type Location struct {
	URL       string `json:"url"`
	PublicURL string `json:"publicUrl"`
	DataCenter string `json:"dataCenter,omitempty"`
}

// UploadResponse represents the response from file upload
type UploadResponse struct {
	Name  string `json:"name,omitempty"`
	Size  int64  `json:"size,omitempty"`
	Error string `json:"error,omitempty"`
	ETag  string `json:"eTag,omitempty"`
}