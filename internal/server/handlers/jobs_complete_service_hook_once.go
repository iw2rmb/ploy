package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/store"
)

var hookHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var hookHashMetadataKeys = []string{
	"hook_hash",
	"hook_once_persistence_key",
}

var hookShouldRunMetadataKeys = []string{
	"hook_should_run",
}

var hookOnceSkipMetadataKeys = []string{
	"hook_once_skip_marked",
	"hook_once_skipped",
	"hook_once_skip",
}

type hookCompletionMetadata struct {
	Hash           string
	ShouldRun      bool
	ShouldRunKnown bool
	OnceSkipMarked bool
}

func (s *CompleteJobService) recordHookOnceLedger(ctx context.Context, state *completeJobState) error {
	if state.serviceType != completeJobServiceTypeHook {
		return nil
	}

	meta, err := parseHookCompletionMetadata(state.input.StatsPayload.Metadata)
	if err != nil {
		return err
	}
	if meta.Hash == "" {
		return nil
	}

	jobID := state.job.ID
	if meta.OnceSkipMarked {
		return s.store.MarkHookOnceSkipped(ctx, store.MarkHookOnceSkippedParams{
			RunID:         state.job.RunID,
			RepoID:        state.job.RepoID,
			HookHash:      meta.Hash,
			LastSkipJobID: &jobID,
		})
	}
	if meta.ShouldRunKnown && !meta.ShouldRun {
		return nil
	}

	return s.store.UpsertHookOnceSuccess(ctx, store.UpsertHookOnceSuccessParams{
		RunID:             state.job.RunID,
		RepoID:            state.job.RepoID,
		HookHash:          meta.Hash,
		FirstSuccessJobID: &jobID,
	})
}

func parseHookCompletionMetadata(metadata map[string]string) (hookCompletionMetadata, error) {
	if len(metadata) == 0 {
		return hookCompletionMetadata{}, nil
	}

	parsed := hookCompletionMetadata{}
	if key, value, ok := firstMetadataValue(metadata, hookHashMetadataKeys...); ok {
		hash := strings.ToLower(strings.TrimSpace(value))
		if !hookHashPattern.MatchString(hash) {
			return hookCompletionMetadata{}, fmt.Errorf("invalid %s metadata value %q: expected 64-char lowercase hex hook hash", key, value)
		}
		parsed.Hash = hash
	}

	if key, value, ok := firstMetadataValue(metadata, hookShouldRunMetadataKeys...); ok {
		b, err := parseMetadataBool(value)
		if err != nil {
			return hookCompletionMetadata{}, fmt.Errorf("invalid %s metadata value %q: %w", key, value, err)
		}
		parsed.ShouldRun = b
		parsed.ShouldRunKnown = true
	}

	if key, value, ok := firstMetadataValue(metadata, hookOnceSkipMetadataKeys...); ok {
		b, err := parseMetadataBool(value)
		if err != nil {
			return hookCompletionMetadata{}, fmt.Errorf("invalid %s metadata value %q: %w", key, value, err)
		}
		parsed.OnceSkipMarked = b
	}

	return parsed, nil
}

func firstMetadataValue(metadata map[string]string, keys ...string) (string, string, bool) {
	for _, key := range keys {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		return key, value, true
	}
	return "", "", false
}

func parseMetadataBool(raw string) (bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false, fmt.Errorf("empty bool")
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, err
	}
	return parsed, nil
}
