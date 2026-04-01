package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

const (
	installedStacksCatalogPath  = "/etc/ploy/gates/stacks.yaml"
	contentTypeJSON             = "application/json"
	defaultRegistryImagePrefix  = "ghcr.io/iw2rmb/ploy"
	containerRegistryEnvVarName = "PLOY_CONTAINER_REGISTRY"
)

type gateCatalogStack struct {
	Language    string
	Release     string
	Tool        string
	Image       string
	ProfileRef  string
	ProfilePath string
}

type gateCatalogSeedStore interface {
	UpsertStackBySelector(ctx context.Context, lang, release, tool, image string) (int64, error)
	UpsertDefaultGateProfile(ctx context.Context, stackID int64, objectKey string) (int64, error)
}

type sqlGateCatalogSeedStore struct {
	pool *pgxpool.Pool
}

func newSQLGateCatalogSeedStore(pool *pgxpool.Pool) gateCatalogSeedStore {
	return &sqlGateCatalogSeedStore{pool: pool}
}

func (s *sqlGateCatalogSeedStore) UpsertStackBySelector(
	ctx context.Context,
	lang, release, tool, image string,
) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
WITH updated AS (
  UPDATE stacks
  SET tool = $3,
      image = $4,
      updated_at = NOW()
  WHERE lang = $1
    AND release = $2
    AND COALESCE(tool, '') = $3
  RETURNING id
),
inserted AS (
  INSERT INTO stacks (lang, release, tool, image)
  SELECT $1, $2, $3, $4
  WHERE NOT EXISTS (SELECT 1 FROM updated)
  RETURNING id
)
SELECT id FROM updated
UNION ALL
SELECT id FROM inserted
LIMIT 1
`, lang, release, tool, image).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert stack (%s,%s,%s): %w", lang, release, tool, err)
	}
	return id, nil
}

func (s *sqlGateCatalogSeedStore) UpsertDefaultGateProfile(
	ctx context.Context,
	stackID int64,
	objectKey string,
) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
WITH updated AS (
  UPDATE gate_profiles
  SET url = $2,
      updated_at = NOW()
  WHERE repo_id IS NULL
    AND repo_sha IS NULL
    AND stack_id = $1
  RETURNING id
),
inserted AS (
  INSERT INTO gate_profiles (repo_id, repo_sha, repo_sha8, stack_id, url)
  SELECT NULL, NULL, NULL, $1, $2
  WHERE NOT EXISTS (SELECT 1 FROM updated)
  RETURNING id
)
SELECT id FROM updated
UNION ALL
SELECT id FROM inserted
LIMIT 1
`, stackID, objectKey).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert default gate profile for stack_id=%d: %w", stackID, err)
	}
	return id, nil
}

func seedGateCatalogDefaults(
	ctx context.Context,
	st gateCatalogSeedStore,
	bs blobstore.Store,
	catalogPath string,
) error {
	if st == nil {
		return fmt.Errorf("seed gate catalog: store is required")
	}
	if bs == nil {
		return fmt.Errorf("seed gate catalog: blobstore is required")
	}
	stacks, err := loadGateCatalogStacks(catalogPath)
	if err != nil {
		return err
	}
	for _, stack := range stacks {
		profileJSON, err := loadGateProfileAsJSON(stack.ProfilePath)
		if err != nil {
			return fmt.Errorf("seed gate catalog: stack %s:%s:%s: %w", stack.Language, stack.Release, stack.Tool, err)
		}
		objectKey := defaultGateProfileObjectKey(stack)
		if _, err := bs.Put(ctx, objectKey, contentTypeJSON, profileJSON); err != nil {
			return fmt.Errorf("seed gate catalog: upload default profile %q: %w", objectKey, err)
		}
		stackID, err := st.UpsertStackBySelector(ctx, stack.Language, stack.Release, stack.Tool, stack.Image)
		if err != nil {
			return err
		}
		if _, err := st.UpsertDefaultGateProfile(ctx, stackID, objectKey); err != nil {
			return err
		}
	}
	return nil
}

func resolveStacksCatalogPath() string {
	if info, err := os.Stat(installedStacksCatalogPath); err == nil && !info.IsDir() {
		return installedStacksCatalogPath
	}

	goModuleFile := "go." + "mo" + "d"
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for {
			if info, serr := os.Stat(filepath.Join(dir, goModuleFile)); serr == nil && !info.IsDir() {
				candidate := filepath.Join(dir, step.DefaultStacksCatalogPath)
				if info, serr := os.Stat(candidate); serr == nil && !info.IsDir() {
					return candidate
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return installedStacksCatalogPath
}

func loadGateCatalogStacks(catalogPath string) ([]gateCatalogStack, error) {
	pathValue := strings.TrimSpace(catalogPath)
	if pathValue == "" {
		pathValue = resolveStacksCatalogPath()
	}
	data, err := os.ReadFile(pathValue)
	if err != nil {
		return nil, fmt.Errorf("seed gate catalog: read %q: %w", pathValue, err)
	}

	var raw struct {
		Stacks []map[string]any `yaml:"stacks"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("seed gate catalog: parse %q: %w", pathValue, err)
	}
	if len(raw.Stacks) == 0 {
		return nil, fmt.Errorf("seed gate catalog: %q has no stacks entries", pathValue)
	}

	stacks := make([]gateCatalogStack, 0, len(raw.Stacks))
	for i, item := range raw.Stacks {
		stack, err := parseGateCatalogStack(pathValue, item, fmt.Sprintf("stacks[%d]", i))
		if err != nil {
			return nil, fmt.Errorf("seed gate catalog: %w", err)
		}
		stacks = append(stacks, stack)
	}
	return stacks, nil
}

func parseGateCatalogStack(catalogPath string, item map[string]any, prefix string) (gateCatalogStack, error) {
	lang, err := requiredStringField(item, "lang", prefix)
	if err != nil {
		return gateCatalogStack{}, err
	}
	releaseValue, ok := item["release"]
	if !ok {
		return gateCatalogStack{}, fmt.Errorf("%s.release: required", prefix)
	}
	release, err := contracts.ParseReleaseValue(releaseValue, prefix+".release")
	if err != nil {
		return gateCatalogStack{}, err
	}
	imageRaw, err := requiredStringField(item, "image", prefix)
	if err != nil {
		return gateCatalogStack{}, err
	}
	profileRef, err := requiredStringField(item, "profile", prefix)
	if err != nil {
		return gateCatalogStack{}, err
	}
	profilePath := resolveCatalogAssetPath(catalogPath, profileRef)
	info, statErr := os.Stat(profilePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return gateCatalogStack{}, fmt.Errorf("%s.profile: referenced file does not exist %q (resolved to %q)", prefix, profileRef, profilePath)
		}
		return gateCatalogStack{}, fmt.Errorf("%s.profile: stat referenced file %q: %w", prefix, profileRef, statErr)
	}
	if info.IsDir() {
		return gateCatalogStack{}, fmt.Errorf("%s.profile: referenced file is a directory %q (resolved to %q)", prefix, profileRef, profilePath)
	}

	tool := ""
	if v, exists := item["tool"]; exists && v != nil {
		tv, ok := v.(string)
		if !ok {
			return gateCatalogStack{}, fmt.Errorf("%s.tool: expected string, got %T", prefix, v)
		}
		tool = strings.TrimSpace(tv)
	}

	return gateCatalogStack{
		Language:    lang,
		Release:     strings.TrimSpace(release),
		Tool:        tool,
		Image:       expandContainerRegistryPrefix(strings.TrimSpace(imageRaw)),
		ProfileRef:  profileRef,
		ProfilePath: profilePath,
	}, nil
}

func requiredStringField(item map[string]any, key, prefix string) (string, error) {
	v, ok := item[key]
	if !ok || v == nil {
		return "", fmt.Errorf("%s.%s: required", prefix, key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s: expected string, got %T", prefix, key, v)
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", fmt.Errorf("%s.%s: required", prefix, key)
	}
	return trimmed, nil
}

func resolveCatalogAssetPath(catalogPath, profileRef string) string {
	cleanRef := path.Clean(strings.TrimSpace(profileRef))
	if filepath.IsAbs(cleanRef) {
		return cleanRef
	}
	catalogDir := filepath.Dir(catalogPath)
	if strings.HasPrefix(cleanRef, "gates/") {
		return filepath.Join(filepath.Dir(catalogDir), filepath.FromSlash(cleanRef))
	}
	return filepath.Join(catalogDir, filepath.FromSlash(cleanRef))
}

func loadGateProfileAsJSON(profilePath string) ([]byte, error) {
	raw, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", profilePath, err)
	}
	var payload any
	if err := yaml.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse profile yaml %q: %w", profilePath, err)
	}
	normalized := normalizeYAMLValue(payload)
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal profile %q to json: %w", profilePath, err)
	}
	if _, err := contracts.ParseGateProfileJSON(data); err != nil {
		return nil, fmt.Errorf("validate profile %q: %w", profilePath, err)
	}
	return data, nil
}

func normalizeYAMLValue(v any) any {
	switch tv := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(tv))
		for k, vv := range tv {
			out[fmt.Sprint(k)] = normalizeYAMLValue(vv)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(tv))
		for k, vv := range tv {
			out[k] = normalizeYAMLValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(tv))
		for i := range tv {
			out[i] = normalizeYAMLValue(tv[i])
		}
		return out
	default:
		return v
	}
}

func defaultGateProfileObjectKey(stack gateCatalogStack) string {
	tool := stack.Tool
	if tool == "" {
		tool = "default"
	}
	return fmt.Sprintf(
		"gate-profiles/defaults/%s/%s/%s/profile.json",
		sanitizeObjectKeyPart(stack.Language),
		sanitizeObjectKeyPart(stack.Release),
		sanitizeObjectKeyPart(tool),
	)
}

func sanitizeObjectKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune('_')
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}

func expandContainerRegistryPrefix(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return image
	}
	prefix := strings.TrimSpace(os.Getenv(containerRegistryEnvVarName))
	if prefix == "" {
		prefix = defaultRegistryImagePrefix
	}
	prefix = strings.TrimRight(prefix, "/")
	expanded := strings.ReplaceAll(image, "${"+containerRegistryEnvVarName+"}", prefix)
	expanded = strings.ReplaceAll(expanded, "$"+containerRegistryEnvVarName, prefix)
	return strings.TrimSpace(expanded)
}
