package grid

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"
)

// fileCursorStore persists workflow stream cursors on disk so resumable streams
// survive CLI restarts.
type fileCursorStore struct {
	path string
	mu   sync.RWMutex
}

var _ helper.CursorStore = (*fileCursorStore)(nil)

func newFileCursorStore(dir, tenant, workflowID, runID string) (helper.CursorStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("cursor store directory is required")
	}
	name := safeCursorFileName(tenant, workflowID, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cursor store: %w", err)
	}
	return &fileCursorStore{path: filepath.Join(dir, name)}, nil
}

func (s *fileCursorStore) Load(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read cursor: %w", err)
	}
	return string(data), nil
}

func (s *fileCursorStore) Store(ctx context.Context, cursor string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cursor == "" {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("delete cursor: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(s.path, []byte(cursor), 0o600); err != nil {
		return fmt.Errorf("write cursor: %w", err)
	}
	return nil
}

func safeCursorFileName(tenant, workflowID, runID string) string {
	parts := []string{tenant, workflowID, runID}
	for i, part := range parts {
		parts[i] = sanitizeCursorSegment(part)
	}
	return fmt.Sprintf("%s_%s_%s.cursor", parts[0], parts[1], parts[2])
}

func sanitizeCursorSegment(value string) string {
	if value == "" {
		return "unknown"
	}
	clean := make([]rune, 0, len(value))
	for _, r := range value {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			clean = append(clean, '-')
		default:
			clean = append(clean, r)
		}
	}
	return string(clean)
}

// NewCursorStoreFactory builds a cursor store factory rooted at the provided directory.
func NewCursorStoreFactory(baseDir string) CursorStoreFactory {
	trimmed := strings.TrimSpace(baseDir)
	return func(tenant, workflowID, runID string) (helper.CursorStore, error) {
		if trimmed == "" {
			return &helper.MemoryCursorStore{}, nil
		}
		cursorDir := filepath.Join(trimmed, "cursors")
		return newFileCursorStore(cursorDir, tenant, workflowID, runID)
	}
}
