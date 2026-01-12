package nodeagent

import (
	"net/http"
)

// baseUploader provides common HTTP client functionality for all uploaders.
// Embed this type in specific uploaders to avoid duplicating client initialization.
type baseUploader struct {
	cfg    Config
	client *http.Client
}

// newBaseUploader creates a new base uploader with an initialized HTTP client.
func newBaseUploader(cfg Config) (*baseUploader, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &baseUploader{
		cfg:    cfg,
		client: client,
	}, nil
}
