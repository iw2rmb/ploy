package storage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SeaweedFSConfig represents SeaweedFS-specific configuration.
type SeaweedFSConfig struct {
	Master      string `yaml:"master"`
	Filer       string `yaml:"filer"`
	Collection  string `yaml:"collection"`
	Replication string `yaml:"replication"`
	Timeout     int    `yaml:"timeout"`
	DataCenter  string `yaml:"datacenter"`
	Rack        string `yaml:"rack"`
}

// SeaweedFSClient implements StorageProvider for SeaweedFS.
type SeaweedFSClient struct {
	masterURL   string
	filerURL    string
	collection  string
	replication string
	timeout     time.Duration
	httpClient  *http.Client
}

var _ StorageProvider = (*SeaweedFSClient)(nil)

// NewSeaweedFSClient creates a new SeaweedFS storage client.
func NewSeaweedFSClient(cfg SeaweedFSConfig) (*SeaweedFSClient, error) {
	if cfg.Master == "" {
		return nil, fmt.Errorf("seaweedfs master address is required")
	}
	if cfg.Filer == "" {
		return nil, fmt.Errorf("seaweedfs filer address is required")
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	collection := cfg.Collection
	if collection == "" {
		collection = "artifacts"
	}

	replication := cfg.Replication
	if replication == "" {
		replication = "000"
	}

	client := &SeaweedFSClient{
		masterURL:   ensureHTTPScheme(cfg.Master),
		filerURL:    ensureHTTPScheme(cfg.Filer),
		collection:  collection,
		replication: replication,
		timeout:     timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	return client, nil
}

func ensureHTTPScheme(addr string) string {
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		return "http://" + addr
	}
	return addr
}

func (c *SeaweedFSClient) GetProviderType() string    { return "seaweedfs" }
func (c *SeaweedFSClient) GetArtifactsBucket() string { return c.collection }

// TestVolumeAssignment verifies that the SeaweedFS master can allocate volumes.
func (c *SeaweedFSClient) TestVolumeAssignment() (map[string]interface{}, error) {
	assignment, err := c.assignVolume()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"fid":       assignment.FileID,
		"url":       assignment.URL,
		"publicUrl": assignment.PublicURL,
		"count":     assignment.Count,
	}, nil
}

type VolumeAssignment struct {
	FileID    string `json:"fid"`
	URL       string `json:"url"`
	PublicURL string `json:"publicUrl"`
	Count     int    `json:"count"`
}

func (c *SeaweedFSClient) assignVolume() (*VolumeAssignment, error) {
	url := fmt.Sprintf("%s/dir/assign?collection=%s&replication=%s", c.masterURL, c.collection, c.replication)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to assign volume: %s", resp.Status)
	}

	var assignment VolumeAssignment
	if err := json.NewDecoder(resp.Body).Decode(&assignment); err != nil {
		return nil, err
	}

	return &assignment, nil
}
