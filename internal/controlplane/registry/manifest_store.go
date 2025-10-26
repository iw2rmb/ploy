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
