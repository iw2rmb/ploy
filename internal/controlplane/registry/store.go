package registry

import (
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	// ErrBlobNotFound indicates the requested blob metadata does not exist.
	ErrBlobNotFound = errors.New("registry: blob not found")
	// ErrManifestNotFound indicates the requested manifest does not exist.
	ErrManifestNotFound = errors.New("registry: manifest not found")
	// ErrTagNotFound indicates the requested tag mapping does not exist.
	ErrTagNotFound = errors.New("registry: tag not found")
)

const (
	defaultRegistryPrefix = "/ploy/registry"

	// BlobStatusAvailable marks a blob as ready for consumption.
	BlobStatusAvailable = "available"
	// BlobStatusDeleted indicates the blob metadata has been removed.
	BlobStatusDeleted = "deleted"
)

// StoreOptions configure the registry metadata store.
type StoreOptions struct {
	Prefix string
	Clock  func() time.Time
}

// Store persists OCI repository metadata (blobs, manifests, and tags) in etcd.
type Store struct {
	client *clientv3.Client
	prefix string
	clock  func() time.Time
}

// NewStore constructs an etcd-backed registry metadata store.
func NewStore(client *clientv3.Client, opts StoreOptions) (*Store, error) {
	if client == nil {
		return nil, errors.New("registry: etcd client required")
	}
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = defaultRegistryPrefix
	}
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Store{client: client, prefix: strings.TrimSuffix(prefix, "/"), clock: clock}, nil
}

func (s *Store) ensureClient() error {
	if s == nil || s.client == nil {
		return errors.New("registry: store uninitialised")
	}
	return nil
}

func (s *Store) validateRepoDigest(repo, digest string) (string, string, error) {
	trimmedRepo := strings.Trim(strings.TrimSpace(repo), "/")
	if trimmedRepo == "" {
		return "", "", errors.New("registry: repo required")
	}
	trimmedDigest := strings.TrimSpace(digest)
	if trimmedDigest == "" {
		return "", "", errors.New("registry: digest required")
	}
	return trimmedRepo, trimmedDigest, nil
}

func (s *Store) blobKey(repo, digest string) string {
	return fmt.Sprintf("%s/repos/%s/blobs/%s", s.prefix, repo, digest)
}

func (s *Store) manifestKey(repo, digest string) string {
	return fmt.Sprintf("%s/repos/%s/manifests/%s", s.prefix, repo, digest)
}

func (s *Store) tagKey(repo, tag string) string {
	return fmt.Sprintf("%s/repos/%s/tags/%s", s.prefix, repo, tag)
}

func (s *Store) tagsPrefix(repo string) string {
	return fmt.Sprintf("%s/repos/%s/tags/", s.prefix, repo)
}
