package step

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ArtifactFetcher retrieves snapshot or diff artifacts from remote storage.
type ArtifactFetcher interface {
	Fetch(ctx context.Context, cid string) (RemoteArtifact, error)
}

// RemoteArtifact represents an artifact payload resolved from remote storage.
type RemoteArtifact struct {
	CID     string
	Digest  string
	Size    int64
	Content io.ReadCloser
}

// Close releases the underlying artifact stream.
func (a RemoteArtifact) Close() error {
	if a.Content != nil {
		return a.Content.Close()
	}
	return nil
}

// RepositoryFetcher clones repositories and produces snapshot artifacts when no CID is supplied.
type RepositoryFetcher interface {
	FetchRepository(ctx context.Context, req RepositoryFetchRequest) (RepositoryFetchResult, error)
}

// RepositoryFetchRequest describes the repository hydration inputs.
type RepositoryFetchRequest struct {
	Repo contracts.RepoMaterialization
}

// RepositoryFetchResult returns the archived repository snapshot.
type RepositoryFetchResult struct {
	Artifact contracts.StepInputArtifactRef
	TarPath  string
	Commit   string
	Ref      string
}

func (h *FilesystemWorkspaceHydrator) hydrateWithPlan(ctx context.Context, input contracts.StepInput, targetDir string) (*PublishedArtifact, error) {
	if h == nil {
		return nil, errors.New("workspace hydrator not configured")
	}
	plan := input.Hydration
	if plan == nil {
		return nil, errors.New("hydration plan missing")
	}

	var baseRef *contracts.StepInputArtifactRef
	if strings.TrimSpace(plan.BaseSnapshot.CID) != "" {
		ref := plan.BaseSnapshot
		baseRef = &ref
	} else if strings.TrimSpace(input.SnapshotCID) != "" {
		baseRef = &contracts.StepInputArtifactRef{CID: strings.TrimSpace(input.SnapshotCID)}
	}

	diffRefs := make([]contracts.StepInputArtifactRef, 0, len(plan.Diffs))
	for _, diff := range plan.Diffs {
		diffRefs = append(diffRefs, diff)
	}
	if len(diffRefs) == 0 && strings.TrimSpace(input.DiffCID) != "" {
		diffRefs = append(diffRefs, contracts.StepInputArtifactRef{CID: strings.TrimSpace(input.DiffCID)})
	}

	var repoRef *contracts.RepoMaterialization
	if plan.Repo != nil && strings.TrimSpace(plan.Repo.URL) != "" {
		repoCopy := *plan.Repo
		repoRef = &repoCopy
	}

	baseExtracted := false
	if baseRef != nil && strings.TrimSpace(baseRef.CID) != "" {
		path, err := h.ensureSnapshot(ctx, *baseRef)
		if err != nil {
			return nil, err
		}
		if err := h.extractArtifact(ctx, path, targetDir); err != nil {
			return nil, err
		}
		baseExtracted = true
	}

	var hydrationArtifact *PublishedArtifact
	if !baseExtracted && repoRef != nil {
		art, err := h.hydrateFromRepo(ctx, *repoRef, targetDir)
		if err != nil {
			return nil, err
		}
		if art != nil {
			hydrationArtifact = art
		}
		baseExtracted = true
	}

	if !baseExtracted {
		return nil, fmt.Errorf("input %s hydration missing base snapshot or repo metadata", input.Name)
	}

	for _, diff := range diffRefs {
		path, err := h.ensureDiff(ctx, diff)
		if err != nil {
			return nil, err
		}
		if err := h.extractArtifact(ctx, path, targetDir); err != nil {
			return nil, err
		}
	}

	return hydrationArtifact, nil
}

func (h *FilesystemWorkspaceHydrator) hydrateFromRepo(ctx context.Context, repo contracts.RepoMaterialization, targetDir string) (*PublishedArtifact, error) {
	if h.repoFetcher == nil {
		return nil, errors.New("repo fetcher not configured")
	}
	result, err := h.repoFetcher.FetchRepository(ctx, RepositoryFetchRequest{Repo: repo})
	if err != nil {
		return nil, err
	}
	tarPath := strings.TrimSpace(result.TarPath)
	if tarPath == "" {
		return nil, errors.New("repo fetcher returned empty tar path")
	}

	var published *PublishedArtifact
	if strings.TrimSpace(result.Artifact.CID) != "" {
		cachedPath, err := h.cacheSnapshotFromExisting(result, tarPath)
		if err != nil {
			return nil, err
		}
		tarPath = cachedPath
		published = &PublishedArtifact{
			CID:    strings.TrimSpace(result.Artifact.CID),
			Digest: strings.TrimSpace(result.Artifact.Digest),
			Size:   result.Artifact.Size,
			Kind:   ArtifactKindSnapshot,
		}
	} else if err := h.writeSnapshotMetadata(tarPath, result); err != nil {
		return nil, err
	}

	if err := h.extractArtifact(ctx, tarPath, targetDir); err != nil {
		return nil, err
	}
	return published, nil
}

func (h *FilesystemWorkspaceHydrator) ensureSnapshot(ctx context.Context, ref contracts.StepInputArtifactRef) (string, error) {
	path := snapshotArtifactPath(h.artifactRoot, ref.CID)
	return h.ensureArtifact(ctx, path, ref)
}

func (h *FilesystemWorkspaceHydrator) ensureDiff(ctx context.Context, ref contracts.StepInputArtifactRef) (string, error) {
	path := diffArtifactPath(h.artifactRoot, ref.CID)
	return h.ensureArtifact(ctx, path, ref)
}

func (h *FilesystemWorkspaceHydrator) ensureArtifact(ctx context.Context, path string, ref contracts.StepInputArtifactRef) (string, error) {
	if strings.TrimSpace(ref.CID) == "" {
		return "", errors.New("artifact cid required")
	}
	unlock := h.lockPath(path)
	defer unlock()

	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		if err := verifyFileDigest(path, ref.Digest); err != nil {
			return "", err
		}
		return path, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if h.fetcher == nil {
		return "", fmt.Errorf("artifact %s missing locally and no fetcher configured", ref.CID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	remote, err := h.fetcher.Fetch(ctx, ref.CID)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = remote.Close()
	}()
	if remote.Content == nil {
		return "", errors.New("artifact fetcher returned nil content")
	}

	temp, err := os.CreateTemp(filepath.Dir(path), "artifact-*")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	writer := io.MultiWriter(temp, hash)
	if _, err := io.Copy(writer, remote.Content); err != nil {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		return "", err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(temp.Name())
		return "", err
	}

	actualDigest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	expected := firstNonEmpty(strings.TrimSpace(ref.Digest), strings.TrimSpace(remote.Digest))
	if expected != "" && !digestsEqual(expected, actualDigest) {
		_ = os.Remove(temp.Name())
		return "", fmt.Errorf("artifact %s digest mismatch: expected %s got %s", ref.CID, expected, actualDigest)
	}

	if err := os.Rename(temp.Name(), path); err != nil {
		_ = os.Remove(temp.Name())
		return "", err
	}
	return path, nil
}

func (h *FilesystemWorkspaceHydrator) cacheSnapshotFromExisting(result RepositoryFetchResult, src string) (string, error) {
	artifact := result.Artifact
	dest := snapshotArtifactPath(h.artifactRoot, artifact.CID)
	unlock := h.lockPath(dest)
	defer unlock()

	if info, err := os.Stat(dest); err == nil && !info.IsDir() {
		if err := verifyFileDigest(dest, artifact.Digest); err != nil {
			return "", err
		}
		return dest, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := copyFile(src, dest); err != nil {
		return "", err
	}
	if err := verifyFileDigest(dest, artifact.Digest); err != nil {
		return "", err
	}
	if err := h.writeSnapshotMetadata(dest, result); err != nil {
		return "", err
	}
	return dest, nil
}

func (h *FilesystemWorkspaceHydrator) lockPath(path string) func() {
	actual, _ := h.locks.LoadOrStore(path, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return func() {
		mu.Unlock()
	}
}

func verifyFileDigest(path, expected string) error {
	trimmed := strings.TrimSpace(expected)
	if trimmed == "" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "sha256:") {
		return fmt.Errorf("unsupported digest %q", trimmed)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if !digestsEqual(trimmed, actual) {
		return fmt.Errorf("digest mismatch for %s: expected %s got %s", filepath.Base(path), trimmed, actual)
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		closeErr := out.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return err
	}
	return out.Close()
}

func digestsEqual(expected, actual string) bool {
	return strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(actual))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (h *FilesystemWorkspaceHydrator) writeSnapshotMetadata(dest string, result RepositoryFetchResult) error {
	commit := strings.TrimSpace(result.Commit)
	ref := strings.TrimSpace(result.Ref)
	if commit == "" && ref == "" {
		return nil
	}
	payload := map[string]string{}
	if commit != "" {
		payload["commit"] = commit
	}
	if ref != "" {
		payload["ref"] = ref
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	metaPath := dest + ".meta"
	return os.WriteFile(metaPath, data, 0o644)
}
