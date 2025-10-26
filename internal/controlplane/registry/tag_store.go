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

// TagDocument links a mutable tag to an immutable manifest digest.
type TagDocument struct {
	Repo      string    `json:"repo"`
	Name      string    `json:"name"`
	Digest    string    `json:"digest"`
	UpdatedAt time.Time `json:"updated_at"`
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
