package artifacts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) read(ctx context.Context, id string) (Metadata, error) {
	meta, _, err := s.readWithRevision(ctx, id)
	return meta, err
}

func (s *Store) readWithRevision(ctx context.Context, id string) (Metadata, int64, error) {
	key := s.artifactKey(strings.TrimSpace(id))
	if key == "" {
		return Metadata{}, 0, ErrNotFound
	}
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return Metadata{}, 0, fmt.Errorf("artifacts: get %s: %w", id, err)
	}
	if len(resp.Kvs) == 0 {
		return Metadata{}, 0, ErrNotFound
	}
	meta, err := s.readRawRecord(resp.Kvs[0].Value)
	if err != nil {
		return Metadata{}, 0, err
	}
	return meta, resp.Kvs[0].ModRevision, nil
}

func (s *Store) readRaw(ctx context.Context, id string) (Metadata, error) {
	meta, _, err := s.readWithRevision(ctx, id)
	return meta, err
}

func (s *Store) readRawRecord(data []byte) (Metadata, error) {
	var rec metadataRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return Metadata{}, fmt.Errorf("artifacts: decode metadata: %w", err)
	}
	return rec.toMetadata(), nil
}
