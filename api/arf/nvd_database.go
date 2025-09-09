package arf

import (
	"context"
	"net/http"
	"time"
)

// NVDDatabase implements CVEDatabase using the National Vulnerability Database
type NVDDatabase struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	cache      map[string]*CVEInfo
}

// NewNVDDatabase creates a new NVD database client
func NewNVDDatabase() *NVDDatabase {
	return &NVDDatabase{
		baseURL: "https://services.nvd.nist.gov/rest/json/cves/2.0",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]*CVEInfo),
	}
}

// SetAPIKey sets the NVD API key for enhanced rate limits
func (n *NVDDatabase) SetAPIKey(apiKey string) {
	n.apiKey = apiKey
}

// UpdateDatabase updates the local CVE database
func (n *NVDDatabase) UpdateDatabase(ctx context.Context) error {
	// For NVD, we don't maintain a local database - we query in real time
	// This method could be used to update cached data or local mirrors
	return nil
}
