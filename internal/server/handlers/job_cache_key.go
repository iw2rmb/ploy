package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	iversion "github.com/iw2rmb/ploy/internal/version"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	cacheDefaultGateImage     = "ubuntu:latest"
	cacheDefaultSBOMRegistry  = "ghcr.io/iw2rmb/ploy"
	cacheImageRegistryEnvKey  = "PLOY_CONTAINER_REGISTRY"
	cacheSBOMMavenCollector   = "sbom-maven"
	cacheSBOMGradleCollector  = "sbom-gradle"
	cacheEmptyLangToolRelease = "||"
)

func computeJobCacheKey(jobType domaintypes.JobType, jobName, jobImage, repoSHAIn string, mergedSpec []byte) (string, error) {
	spec, err := contracts.ParseMigSpecJSON(mergedSpec)
	if err != nil {
		// Claim-time replay must fail open when spec cannot be parsed.
		return "", nil
	}

	resolvedImage := strings.TrimSpace(jobImage)
	if resolvedImage == "" {
		resolvedImage = resolveJobImageForCacheKey(jobType, jobName, spec)
	}
	if resolvedImage == "" {
		// No deterministic image means no safe cache key for this claim.
		return "", nil
	}

	envPairs, err := resolveEnvPairsForCacheKey(jobType, jobName, spec)
	if err != nil {
		return "", err
	}

	inEntries, homeEntries, caEntries, err := resolveHydraForCacheKey(jobType, jobName, spec)
	if err != nil {
		return "", err
	}

	inCanonical, err := canonicalHydraHashDstList(inEntries, false)
	if err != nil {
		return "", err
	}
	homeCanonical, err := canonicalHydraHashDstList(homeEntries, true)
	if err != nil {
		return "", err
	}
	caCanonical, err := canonicalCAHashes(caEntries)
	if err != nil {
		return "", err
	}

	ltr := resolveLangToolReleaseForCacheKey(jobType, jobName, spec)
	payload := strings.Join([]string{
		strings.TrimSpace(jobType.String()),
		resolvedImage,
		normalizeRepoSHA(repoSHAIn),
		ltr,
		strings.Join(envPairs, ";"),
		strings.Join(inCanonical, ";"),
		strings.Join(homeCanonical, ";"),
		strings.Join(caCanonical, ";"),
	}, "\n")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:]), nil
}

func resolveJobImageForCacheKey(jobType domaintypes.JobType, jobName string, spec *contracts.MigSpec) string {
	switch jobType {
	case domaintypes.JobTypeMig:
		if spec == nil || len(spec.Steps) == 0 {
			return ""
		}
		idx, err := migStepIndexFromJobNameForClaim(jobName, len(spec.Steps))
		if err != nil || idx < 0 || idx >= len(spec.Steps) {
			return ""
		}
		return canonicalJobImageForCacheKey(spec.Steps[idx].Image)
	case domaintypes.JobTypeHeal:
		if spec == nil || spec.BuildGate == nil || spec.BuildGate.Heal == nil {
			return ""
		}
		return canonicalJobImageForCacheKey(spec.BuildGate.Heal.Image)
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		return cacheDefaultGateImage
	case domaintypes.JobTypeSBOM:
		return resolveSBOMImageForCacheKey(jobType, jobName, spec)
	default:
		return ""
	}
}

func canonicalJobImageForCacheKey(img contracts.JobImage) string {
	if strings.TrimSpace(img.Universal) != "" {
		return strings.TrimSpace(img.Universal)
	}
	if len(img.ByStack) == 0 {
		return ""
	}
	keys := make([]string, 0, len(img.ByStack))
	for k := range img.ByStack {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(img.ByStack[contracts.MigStack(k)])
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "|")
}

func resolveSBOMImageForCacheKey(jobType domaintypes.JobType, jobName string, spec *contracts.MigSpec) string {
	phase := phaseConfigForJobCacheKey(jobType, jobName, spec)
	tool := ""
	if phase != nil && phase.Stack != nil && phase.Stack.Enabled {
		tool = strings.ToLower(strings.TrimSpace(phase.Stack.Tool))
	}

	collector := cacheSBOMMavenCollector
	if tool == "gradle" {
		collector = cacheSBOMGradleCollector
	}

	registry := strings.TrimRight(strings.TrimSpace(os.Getenv(cacheImageRegistryEnvKey)), "/")
	if registry == "" {
		registry = cacheDefaultSBOMRegistry
	}
	tag := strings.TrimSpace(iversion.Version)
	if tag == "" || strings.EqualFold(tag, "dev") {
		tag = "latest"
	}
	return registry + "/" + collector + ":" + tag
}

func resolveEnvPairsForCacheKey(jobType domaintypes.JobType, jobName string, spec *contracts.MigSpec) ([]string, error) {
	env := map[string]string{}
	if spec != nil {
		for k, v := range spec.Envs {
			env[k] = v
		}
	}

	switch jobType {
	case domaintypes.JobTypeMig:
		if spec == nil || len(spec.Steps) == 0 {
			break
		}
		idx, err := migStepIndexFromJobNameForClaim(jobName, len(spec.Steps))
		if err != nil {
			return nil, err
		}
		for k, v := range spec.Steps[idx].Envs {
			env[k] = v
		}
	case domaintypes.JobTypeHeal:
		if spec != nil && spec.BuildGate != nil && spec.BuildGate.Heal != nil {
			for k, v := range spec.BuildGate.Heal.Envs {
				env[k] = v
			}
		}
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out, nil
}

func resolveHydraForCacheKey(jobType domaintypes.JobType, jobName string, spec *contracts.MigSpec) (inEntries []string, homeEntries []string, caEntries []string, err error) {
	if spec == nil {
		return nil, nil, nil, nil
	}

	switch jobType {
	case domaintypes.JobTypeMig:
		if len(spec.Steps) == 0 {
			return nil, nil, nil, nil
		}
		idx, idxErr := migStepIndexFromJobNameForClaim(jobName, len(spec.Steps))
		if idxErr != nil {
			return nil, nil, nil, idxErr
		}
		step := spec.Steps[idx]
		return append([]string(nil), step.In...), append([]string(nil), step.Home...), append([]string(nil), step.CA...), nil
	case domaintypes.JobTypeHeal:
		if spec.BuildGate == nil || spec.BuildGate.Heal == nil {
			return nil, nil, nil, nil
		}
		heal := spec.BuildGate.Heal
		return append([]string(nil), heal.In...), append([]string(nil), heal.Home...), append([]string(nil), heal.CA...), nil
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate, domaintypes.JobTypeSBOM, domaintypes.JobTypeHook:
		phase := phaseConfigForJobCacheKey(jobType, jobName, spec)
		if phase == nil {
			return nil, nil, nil, nil
		}
		return nil, nil, append([]string(nil), phase.CA...), nil
	default:
		return nil, nil, nil, nil
	}
}

func phaseConfigForJobCacheKey(jobType domaintypes.JobType, jobName string, spec *contracts.MigSpec) *contracts.BuildGatePhaseConfig {
	if spec == nil || spec.BuildGate == nil {
		return nil
	}
	switch jobType {
	case domaintypes.JobTypePreGate:
		return spec.BuildGate.Pre
	case domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		return spec.BuildGate.Post
	case domaintypes.JobTypeSBOM, domaintypes.JobTypeHook:
		ctx, ok := inferLegacySBOMCycleContext(store.Job{Name: jobName})
		if ok && ctx.Phase == contracts.SBOMPhasePre {
			return spec.BuildGate.Pre
		}
		return spec.BuildGate.Post
	default:
		return spec.BuildGate.Pre
	}
}

func resolveLangToolReleaseForCacheKey(jobType domaintypes.JobType, jobName string, spec *contracts.MigSpec) string {
	phase := phaseConfigForJobCacheKey(jobType, jobName, spec)
	if phase == nil || phase.Stack == nil || !phase.Stack.Enabled {
		return cacheEmptyLangToolRelease
	}
	lang := strings.ToLower(strings.TrimSpace(phase.Stack.Language))
	tool := strings.ToLower(strings.TrimSpace(phase.Stack.Tool))
	release := strings.TrimSpace(phase.Stack.Release)
	return lang + "|" + tool + "|" + release
}

func canonicalHydraHashDstList(entries []string, home bool) ([]string, error) {
	grouped := map[string][]string{}
	for _, raw := range entries {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if home {
			parsed, err := contracts.ParseStoredHomeEntry(trimmed)
			if err != nil {
				return nil, fmt.Errorf("parse home entry %q: %w", trimmed, err)
			}
			grouped[parsed.Hash] = append(grouped[parsed.Hash], parsed.Dst)
			continue
		}
		parsed, err := contracts.ParseStoredInEntry(trimmed)
		if err != nil {
			return nil, fmt.Errorf("parse in entry %q: %w", trimmed, err)
		}
		grouped[parsed.Hash] = append(grouped[parsed.Hash], parsed.Dst)
	}

	hashes := make([]string, 0, len(grouped))
	for hash := range grouped {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)

	out := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		dsts := uniqueSorted(grouped[hash])
		out = append(out, hash+":"+strings.Join(dsts, ","))
	}
	return out, nil
}

func canonicalCAHashes(entries []string) ([]string, error) {
	hashes := make([]string, 0, len(entries))
	for _, raw := range entries {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		hash, err := contracts.ParseStoredCAEntry(trimmed)
		if err != nil {
			return nil, fmt.Errorf("parse ca entry %q: %w", trimmed, err)
		}
		hashes = append(hashes, hash)
	}
	return uniqueSorted(hashes), nil
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}
