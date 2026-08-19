package speccatalog

import (
	"bytes"
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/iw2rmb/ploy/internal/specdiscovery"
)

func (r *repository) scan(ctx context.Context, sha string, committedAt time.Time) ([]Entry, error) {
	paths, err := r.runGit(ctx, r.checkout, nil, "ls-files", "-z", "--cached", "--", "*.yaml")
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for _, rawPath := range bytes.Split(paths, []byte{0}) {
		if len(rawPath) == 0 {
			continue
		}
		if !utf8.Valid(rawPath) {
			return nil, fmt.Errorf("scan spec repository: tracked YAML path is not valid UTF-8")
		}
		path := string(rawPath)
		content, err := r.runGit(ctx, r.checkout, nil, "show", "HEAD:"+path)
		if err != nil {
			return nil, fmt.Errorf("read tracked YAML %s: %w", path, err)
		}
		probe, ok := specdiscovery.ProbeYAML(content)
		if !ok {
			continue
		}
		entries = append(entries, Entry{
			Name:        probe.Name,
			Description: probe.Description,
			Source:      r.source,
			Path:        path,
			SHA:         sha,
			CommittedAt: committedAt,
			repository:  r,
		})
	}
	return entries, nil
}
