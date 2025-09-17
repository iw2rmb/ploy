package seaweedfs

import (
	"net/http"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// Provider implements both storage.Storage and storage.StorageProvider interfaces for SeaweedFS
type Provider struct {
	masterURL   string
	filerURL    string
	collection  string
	replication string
	timeout     time.Duration
	httpClient  *http.Client
}

// Ensure Provider implements both interfaces
var _ storage.Storage = (*Provider)(nil)
var _ storage.StorageProvider = (*Provider)(nil)
