package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

// BlobDocument captures metadata for a single blob digest.
type BlobDocument struct {
	Repo      string    `json:"repo"`
	Digest    string    `json:"digest"`
	MediaType string    `json:"media_type"`
	Size      int64     `json:"size"`
	CID       string    `json:"cid"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	DeletedAt time.Time `json:"deleted_at,omitempty"`
}

// ManifestDocument stores the content and descriptor references for an OCI manifest.
type ManifestDocument struct {
	Repo         string    `json:"repo"`
	Digest       string    `json:"digest"`
	MediaType    string    `json:"media_type"`
	Size         int64     `json:"size"`
	Payload      []byte    `json:"payload"`
	ConfigDigest string    `json:"config_digest"`
	LayerDigests []string  `json:"layer_digests"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	DeletedAt    time.Time `json:"deleted_at,omitempty"`
}

// TagDocument links a mutable tag to an immutable manifest digest.
type TagDocument struct {
	Repo      string    `json:"repo"`
	Name      string    `json:"name"`
	Digest    string    `json:"digest"`
	UpdatedAt time.Time `json:"updated_at"`
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

// PutBlob inserts or updates blob metadata for the specified repository.
func (s *Store) PutBlob(ctx context.Context, blob BlobDocument) (BlobDocument, error) {
	if err := s.ensureClient(); err != nil {
		return BlobDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(blob.Repo, blob.Digest)
	if err != nil {
		return BlobDocument{}, err
	}
	now := s.clock().UTC()
	existing, err := s.readBlob(ctx, repo, digest)
	if err == nil {
		blob.CreatedAt = existing.CreatedAt
	} else if errors.Is(err, ErrBlobNotFound) {
		blob.CreatedAt = now
	} else if err != nil {
		return BlobDocument{}, err
	}
	blob.Repo = repo
	blob.Digest = digest
	blob.UpdatedAt = now
	blob.DeletedAt = time.Time{}
	if strings.TrimSpace(blob.Status) == "" {
		blob.Status = BlobStatusAvailable
	}
	payload, err := json.Marshal(blob)
	if err != nil {
		return BlobDocument{}, fmt.Errorf("registry: encode blob %s: %w", digest, err)
	}
	if _, err := s.client.Put(ctx, s.blobKey(repo, digest), string(payload)); err != nil {
		return BlobDocument{}, fmt.Errorf("registry: persist blob %s: %w", digest, err)
	}
	return blob, nil
}

// GetBlob fetches blob metadata.
func (s *Store) GetBlob(ctx context.Context, repo, digest string) (BlobDocument, error) {
	if err := s.ensureClient(); err != nil {
		return BlobDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	return s.readBlob(ctx, repo, digest)
}

// DeleteBlob marks blob metadata as deleted.
func (s *Store) DeleteBlob(ctx context.Context, repo, digest string) (BlobDocument, error) {
	if err := s.ensureClient(); err != nil {
		return BlobDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	record, err := s.readBlob(ctx, repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	record.Status = BlobStatusDeleted
	record.UpdatedAt = s.clock().UTC()
	record.DeletedAt = record.UpdatedAt
	payload, err := json.Marshal(record)
	if err != nil {
		return BlobDocument{}, fmt.Errorf("registry: encode blob delete %s: %w", digest, err)
	}
	if _, err := s.client.Put(ctx, s.blobKey(repo, digest), string(payload)); err != nil {
		return BlobDocument{}, fmt.Errorf("registry: delete blob %s: %w", digest, err)
	}
	return record, nil
}

// PutManifest inserts or updates a manifest and optionally links a tag reference.
func (s *Store) PutManifest(ctx context.Context, manifest ManifestDocument, tag string) (ManifestDocument, error) {
	if err := s.ensureClient(); err != nil {
		return ManifestDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(manifest.Repo, manifest.Digest)
	if err != nil {
		return ManifestDocument{}, err
	}
	if len(manifest.Payload) == 0 {
		return ManifestDocument{}, errors.New("registry: manifest payload required")
	}
	for _, required := range collectManifestDigests(manifest) {
		if _, err := s.readBlob(ctx, repo, required); err != nil {
			return ManifestDocument{}, err
		}
	}
	now := s.clock().UTC()
	existing, err := s.readManifest(ctx, repo, digest)
	if err == nil {
		manifest.CreatedAt = existing.CreatedAt
	} else if errors.Is(err, ErrManifestNotFound) {
		manifest.CreatedAt = now
	} else {
		return ManifestDocument{}, err
	}
	manifest.Repo = repo
	manifest.Digest = digest
	manifest.Size = firstNonZero(manifest.Size, int64(len(manifest.Payload)))
	manifest.UpdatedAt = now
	payload, err := json.Marshal(manifest)
	if err != nil {
		return ManifestDocument{}, fmt.Errorf("registry: encode manifest %s: %w", digest, err)
	}
	ops := []clientv3.Op{clientv3.OpPut(s.manifestKey(repo, digest), string(payload))}
	trimmedTag := strings.TrimSpace(tag)
	if trimmedTag != "" {
		tagDoc := TagDocument{Repo: repo, Name: trimmedTag, Digest: digest, UpdatedAt: now}
		tagPayload, err := json.Marshal(tagDoc)
		if err != nil {
			return ManifestDocument{}, fmt.Errorf("registry: encode tag %s: %w", trimmedTag, err)
		}
		ops = append(ops, clientv3.OpPut(s.tagKey(repo, trimmedTag), string(tagPayload)))
	}
	if _, err := s.client.Txn(ctx).Then(ops...).Commit(); err != nil {
		return ManifestDocument{}, fmt.Errorf("registry: persist manifest %s: %w", digest, err)
	}
	return manifest, nil
}

// GetManifest returns a manifest by digest.
func (s *Store) GetManifest(ctx context.Context, repo, digest string) (ManifestDocument, error) {
	if err := s.ensureClient(); err != nil {
		return ManifestDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(repo, digest)
	if err != nil {
		return ManifestDocument{}, err
	}
	return s.readManifest(ctx, repo, digest)
}

// ResolveManifest resolves either a digest or tag reference to a manifest.
func (s *Store) ResolveManifest(ctx context.Context, repo, reference string) (ManifestDocument, error) {
	if strings.Contains(reference, ":") {
		return s.GetManifest(ctx, repo, reference)
	}
	tag, err := s.GetTag(ctx, repo, reference)
	if err != nil {
		return ManifestDocument{}, err
	}
	return s.GetManifest(ctx, repo, tag.Digest)
}

// DeleteManifest removes a manifest and any tags pointing to it.
func (s *Store) DeleteManifest(ctx context.Context, repo, digest string) error {
	if err := s.ensureClient(); err != nil {
		return err
	}
	repo, digest, err := s.validateRepoDigest(repo, digest)
	if err != nil {
		return err
	}
	if _, err := s.readManifest(ctx, repo, digest); err != nil {
		return err
	}
	if _, err := s.client.Delete(ctx, s.manifestKey(repo, digest)); err != nil {
		return fmt.Errorf("registry: delete manifest %s: %w", digest, err)
	}
	tags, err := s.ListTags(ctx, repo)
	if err != nil {
		return err
	}
	for _, tag := range tags {
		if tag.Digest == digest {
			_ = s.DeleteTag(ctx, repo, tag.Name)
		}
	}
	return nil
}

// GetTag fetches a tag document.
func (s *Store) GetTag(ctx context.Context, repo, name string) (TagDocument, error) {
	if err := s.ensureClient(); err != nil {
		return TagDocument{}, err
	}
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return TagDocument{}, errors.New("registry: repo required")
	}
	tag := strings.TrimSpace(name)
	if tag == "" {
		return TagDocument{}, errors.New("registry: tag required")
	}
	resp, err := s.client.Get(ctx, s.tagKey(repo, tag))
	if err != nil {
		return TagDocument{}, fmt.Errorf("registry: get tag %s: %w", tag, err)
	}
	if len(resp.Kvs) == 0 {
		return TagDocument{}, ErrTagNotFound
	}
	var doc TagDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return TagDocument{}, fmt.Errorf("registry: decode tag %s: %w", tag, err)
	}
	return doc, nil
}

// ListTags returns tag mappings for a repository ordered by tag name.
func (s *Store) ListTags(ctx context.Context, repo string) ([]TagDocument, error) {
	if err := s.ensureClient(); err != nil {
		return nil, err
	}
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return nil, errors.New("registry: repo required")
	}
	resp, err := s.client.Get(ctx, s.tagsPrefix(repo), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("registry: list tags: %w", err)
	}
	tags := make([]TagDocument, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var doc TagDocument
		if err := json.Unmarshal(kv.Value, &doc); err != nil {
			return nil, fmt.Errorf("registry: decode tag: %w", err)
		}
		tags = append(tags, doc)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	return tags, nil
}

// DeleteTag removes a tag mapping.
func (s *Store) DeleteTag(ctx context.Context, repo, tag string) error {
	if err := s.ensureClient(); err != nil {
		return err
	}
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return errors.New("registry: repo required")
	}
	trimmedTag := strings.TrimSpace(tag)
	if trimmedTag == "" {
		return errors.New("registry: tag required")
	}
	if _, err := s.client.Delete(ctx, s.tagKey(repo, trimmedTag)); err != nil {
		return fmt.Errorf("registry: delete tag %s: %w", trimmedTag, err)
	}
	return nil
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

func (s *Store) readBlob(ctx context.Context, repo, digest string) (BlobDocument, error) {
	resp, err := s.client.Get(ctx, s.blobKey(repo, digest))
	if err != nil {
		return BlobDocument{}, fmt.Errorf("registry: get blob %s: %w", digest, err)
	}
	if len(resp.Kvs) == 0 {
		return BlobDocument{}, ErrBlobNotFound
	}
	var doc BlobDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return BlobDocument{}, fmt.Errorf("registry: decode blob %s: %w", digest, err)
	}
	if doc.Status == BlobStatusDeleted || !doc.DeletedAt.IsZero() {
		return BlobDocument{}, ErrBlobNotFound
	}
	return doc, nil
}

func (s *Store) readManifest(ctx context.Context, repo, digest string) (ManifestDocument, error) {
	resp, err := s.client.Get(ctx, s.manifestKey(repo, digest))
	if err != nil {
		return ManifestDocument{}, fmt.Errorf("registry: get manifest %s: %w", digest, err)
	}
	if len(resp.Kvs) == 0 {
		return ManifestDocument{}, ErrManifestNotFound
	}
	var doc ManifestDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return ManifestDocument{}, fmt.Errorf("registry: decode manifest %s: %w", digest, err)
	}
	if !doc.DeletedAt.IsZero() {
		return ManifestDocument{}, ErrManifestNotFound
	}
	return doc, nil
}

func collectManifestDigests(manifest ManifestDocument) []string {
	set := make(map[string]struct{})
	for _, digest := range manifest.LayerDigests {
		trimmed := strings.TrimSpace(digest)
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if trimmed := strings.TrimSpace(manifest.ConfigDigest); trimmed != "" {
		set[trimmed] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for digest := range set {
		result = append(result, digest)
	}
	sort.Strings(result)
	return result
}

func firstNonZero(values ...int64) int64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
