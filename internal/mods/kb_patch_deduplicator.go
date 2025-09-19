package mods

import "context"

// DeduplicatePatches finds and merges similar patches.
func (pd *PatchDeduplicator) DeduplicatePatches(ctx context.Context, fingerprints []string) (*DeduplicationStats, error) {
	stats := &DeduplicationStats{}

	if len(fingerprints) < 2 {
		return stats, nil
	}

	patches := make(map[string][]byte)
	for _, fingerprint := range fingerprints {
		patch, err := pd.storage.GetPatch(ctx, fingerprint)
		if err != nil {
			continue
		}
		patches[fingerprint] = patch
	}

	duplicateGroups := pd.findSimilarPatches(patches)

	for _, group := range duplicateGroups {
		if len(group) <= 1 {
			continue
		}

		canonical := group[0]
		for _, fingerprint := range group[1:] {
			if len(patches[fingerprint]) < len(patches[canonical]) {
				canonical = fingerprint
			}
		}

		stats.PatchesDeduplicated += len(group) - 1

		// TODO: Update references to reuse the canonical patch when storage layout supports redirects.
		// For now we only record statistics; actual redirects require richer storage metadata.
	}

	return stats, nil
}

func (pd *PatchDeduplicator) findSimilarPatches(patches map[string][]byte) [][]string {
	var groups [][]string
	processed := make(map[string]bool)

	for fingerprint, patch := range patches {
		if processed[fingerprint] {
			continue
		}

		group := []string{fingerprint}
		processed[fingerprint] = true

		for otherFingerprint, otherPatch := range patches {
			if processed[otherFingerprint] || fingerprint == otherFingerprint {
				continue
			}

			if len(patch) == 0 || len(otherPatch) == 0 {
				continue
			}

			similarity := pd.sigGenerator.ComputePatchSimilarity(patch, otherPatch)
			if similarity >= pd.config.SimilarityThresholdForMerge {
				group = append(group, otherFingerprint)
				processed[otherFingerprint] = true
			}
		}

		if len(group) > 1 {
			groups = append(groups, group)
		}
	}

	return groups
}
