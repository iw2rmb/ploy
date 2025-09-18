package nvd

// moved from nvd_database.go

import (
	"context"
	"net/http"
	"time"

	"github.com/iw2rmb/ploy/api/security"
)

// NVDDatabase implements CVEDatabase using the National Vulnerability Database
type NVDDatabase struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	cache      map[string]*security.CVEInfo
}

// NewNVDDatabase creates a new NVD database client
func NewNVDDatabase() *NVDDatabase {
	return &NVDDatabase{
		baseURL: "https://services.nvd.nist.gov/rest/json/cves/2.0",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]*security.CVEInfo),
	}
}

// SetAPIKey sets the NVD API key for enhanced rate limits
func (n *NVDDatabase) SetAPIKey(apiKey string) {
	n.apiKey = apiKey
}

// SetBaseURL overrides the default NVD API base URL
func (n *NVDDatabase) SetBaseURL(url string) {
	if url != "" {
		n.baseURL = url
	}
}

// SetHTTPTimeout sets the HTTP client timeout
func (n *NVDDatabase) SetHTTPTimeout(d time.Duration) {
	if n.httpClient != nil && d > 0 {
		n.httpClient.Timeout = d
	}
}

// UpdateDatabase updates the local CVE database
func (n *NVDDatabase) UpdateDatabase(ctx context.Context) error {
	// For NVD, we don't maintain a local database - we query in real time
	// This method could be used to update cached data or local mirrors
	return nil
}
