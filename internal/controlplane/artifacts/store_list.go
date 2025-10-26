package artifacts

import (
	"context"
	"fmt"
	"path"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func (s *Store) listByJob(ctx context.Context, jobID, stage, cursor string, limit int, includeDeleted bool) (ListResult, error) {
	prefix := s.jobIndexPrefix(jobID)
	if stage != "" {
		prefix = s.stageIndexPrefix(jobID, stage)
	}
	start := prefix
	if cursor != "" {
		start = path.Join(prefix, cursor)
	}
	resp, err := s.client.Get(ctx, start,
		clientv3.WithRange(prefixRangeEnd(prefix)),
		clientv3.WithLimit(int64(limit+1)),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
	)
	if err != nil {
		return ListResult{}, fmt.Errorf("artifacts: list job %s: %w", jobID, err)
	}
	kvs := dropCursor(resp.Kvs, cursor, prefix)
	return s.collectFromIndex(ctx, kvs, limit, resp.More, includeDeleted)
}

func (s *Store) listByCID(ctx context.Context, cid, cursor string, limit int, includeDeleted bool) (ListResult, error) {
	prefix := s.cidIndexPrefix(cid)
	start := prefix
	if cursor != "" {
		start = path.Join(prefix, cursor)
	}
	resp, err := s.client.Get(ctx, start,
		clientv3.WithRange(prefixRangeEnd(prefix)),
		clientv3.WithLimit(int64(limit+1)),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
	)
	if err != nil {
		return ListResult{}, fmt.Errorf("artifacts: list cid %s: %w", cid, err)
	}
	kvs := dropCursor(resp.Kvs, cursor, prefix)
	return s.collectFromIndex(ctx, kvs, limit, resp.More, includeDeleted)
}

func (s *Store) listAll(ctx context.Context, cursor string, limit int, includeDeleted bool) (ListResult, error) {
	prefix := s.artifactPrefix()
	start := prefix
	if cursor != "" {
		start = path.Join(prefix, cursor)
	}
	resp, err := s.client.Get(ctx, start,
		clientv3.WithRange(prefixRangeEnd(prefix)),
		clientv3.WithLimit(int64(limit+1)),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	if err != nil {
		return ListResult{}, fmt.Errorf("artifacts: list: %w", err)
	}
	kvs := dropCursor(resp.Kvs, cursor, prefix)
	results := make([]Metadata, 0, len(kvs))
	var lastProcessedID string
	for _, kv := range kvs {
		id := artifactIDFromKey(string(kv.Key))
		lastProcessedID = id
		meta, err := s.readRawRecord(kv.Value)
		if err != nil {
			continue
		}
		if meta.Deleted && !includeDeleted {
			continue
		}
		results = append(results, meta)
		if len(results) == limit {
			break
		}
	}
	nextCursor := ""
	if (resp.More || len(results) == limit) && lastProcessedID != "" {
		nextCursor = lastProcessedID
	}
	return ListResult{Artifacts: results, NextCursor: nextCursor}, nil
}

func (s *Store) collectFromIndex(ctx context.Context, kvs []*mvccpb.KeyValue, limit int, hasMore bool, includeDeleted bool) (ListResult, error) {
	results := make([]Metadata, 0, len(kvs))
	var lastProcessedID string
	for _, kv := range kvs {
		id := artifactIDFromKey(string(kv.Key))
		if id == "" {
			continue
		}
		lastProcessedID = id
		meta, err := s.readRaw(ctx, id)
		if err != nil {
			continue
		}
		if meta.Deleted && !includeDeleted {
			continue
		}
		results = append(results, meta)
		if len(results) == limit {
			break
		}
	}
	nextCursor := ""
	if (hasMore || len(results) == limit) && lastProcessedID != "" {
		nextCursor = lastProcessedID
	}
	return ListResult{Artifacts: results, NextCursor: nextCursor}, nil
}
