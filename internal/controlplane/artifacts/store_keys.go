package artifacts

import (
	"net/url"
	"path"
	"strings"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func (s *Store) artifactPrefix() string {
	return path.Join(s.prefix, "artifacts")
}

func (s *Store) artifactKey(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}
	return path.Join(s.artifactPrefix(), trimmed)
}

func (s *Store) jobIndexPrefix(jobID string) string {
	return path.Join(s.prefix, "index", "artifacts", "jobs", encodeSegment(jobID), "artifacts")
}

func (s *Store) jobIndexKey(jobID, id string) string {
	return path.Join(s.jobIndexPrefix(jobID), strings.TrimSpace(id))
}

func (s *Store) stageIndexPrefix(jobID, stage string) string {
	return path.Join(s.prefix, "index", "artifacts", "jobs", encodeSegment(jobID), "stages", encodeSegment(stage))
}

func (s *Store) stageIndexKey(jobID, stage, id string) string {
	return path.Join(s.stageIndexPrefix(jobID, stage), strings.TrimSpace(id))
}

func encodeSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "_"
	}
	return url.PathEscape(trimmed)
}

func artifactIDFromKey(key string) string {
	idx := strings.LastIndex(key, "/")
	if idx == -1 {
		return key
	}
	return key[idx+1:]
}

func dropCursor(kvs []*mvccpb.KeyValue, cursor, prefix string) []*mvccpb.KeyValue {
	if cursor == "" || len(kvs) == 0 {
		return kvs
	}
	cursorKey := path.Join(prefix, cursor)
	if string(kvs[0].Key) == cursorKey {
		return kvs[1:]
	}
	return kvs
}

func prefixRangeEnd(prefix string) string {
	return clientv3.GetPrefixRangeEnd(prefix)
}
