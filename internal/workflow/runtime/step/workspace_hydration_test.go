package step

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestFilesystemWorkspaceHydratorFetchesRemoteSnapshotsAndDiffs(t *testing.T) {
	t.Helper()
	root := t.TempDir()

	snapshotBytes, snapshotDigest := tarBytes(t, map[string]string{
		"README.md": "baseline\n",
	})
	diffBytes, diffDigest := tarBytes(t, map[string]string{
		"README.md": "baseline\noverlay\n",
		"new.txt":   "diff-content\n",
	})

	fetcher := &fakeArtifactFetcher{
		payloads: map[string]testArtifact{
			"bafy-base": {
				data:   snapshotBytes,
				digest: snapshotDigest,
			},
			"bafy-diff": {
				data:   diffBytes,
				digest: diffDigest,
			},
		},
	}

	hydrator, err := NewFilesystemWorkspaceHydrator(FilesystemWorkspaceHydratorOptions{
		ArtifactRoot: root,
		Fetcher:      fetcher,
	})
	if err != nil {
		t.Fatalf("NewFilesystemWorkspaceHydrator: %v", err)
	}

	manifest := contracts.StepManifest{
		ID:    "mods-apply",
		Name:  "Mods Apply",
		Image: "ghcr.io/ploy/mods/apply:latest",
		Inputs: []contracts.StepInput{
			{
				Name:      "baseline",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadOnly,
				Hydration: &contracts.StepInputHydration{
					BaseSnapshot: contracts.StepInputArtifactRef{
						CID:    "bafy-base",
						Digest: snapshotDigest,
					},
					Diffs: []contracts.StepInputArtifactRef{
						{CID: "bafy-diff", Digest: diffDigest},
					},
				},
			},
		},
	}

	ws, err := hydrator.Prepare(context.Background(), WorkspaceRequest{Manifest: manifest})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}

	path := ws.Inputs["baseline"]
	content, err := os.ReadFile(filepath.Join(path, "README.md"))
	if err != nil {
		t.Fatalf("read hydrated file: %v", err)
	}
	if string(content) != "baseline\noverlay\n" {
		t.Fatalf("unexpected hydrated content: %q", string(content))
	}
	diffContent, err := os.ReadFile(filepath.Join(path, "new.txt"))
	if err != nil {
		t.Fatalf("read diff file: %v", err)
	}
	if string(diffContent) != "diff-content\n" {
		t.Fatalf("unexpected diff content: %q", string(diffContent))
	}

	if fetcher.calls["bafy-base"] != 1 || fetcher.calls["bafy-diff"] != 1 {
		t.Fatalf("expected fetch calls for both artifacts, got %+v", fetcher.calls)
	}
	if _, err := os.Stat(filepath.Join(root, "snapshots", sanitizeName("bafy-base"))); err != nil {
		t.Fatalf("expected cached snapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "diffs", sanitizeName("bafy-diff"))); err != nil {
		t.Fatalf("expected cached diff: %v", err)
	}
	if len(ws.HydrationSnapshots) != 0 {
		t.Fatalf("expected no hydration snapshots, got %+v", ws.HydrationSnapshots)
	}
}

func TestFilesystemWorkspaceHydratorClonesRepositoryWhenNoCID(t *testing.T) {
	t.Helper()
	root := t.TempDir()

	tarRoot := t.TempDir()
	tarPath, tarDigest := createTarFile(t, tarRoot, "bafy-clone", map[string]string{
		"main.go": "package main\n",
	})
	info, err := os.Stat(tarPath)
	if err != nil {
		t.Fatalf("stat tar file: %v", err)
	}
	git := &fakeRepoFetcher{
		result: RepositoryFetchResult{
			Artifact: contracts.StepInputArtifactRef{
				CID:    "bafy-clone",
				Digest: tarDigest,
				Size:   info.Size(),
			},
			TarPath: tarPath,
			Commit:  "abc123",
		},
	}

	hydrator, err := NewFilesystemWorkspaceHydrator(FilesystemWorkspaceHydratorOptions{
		ArtifactRoot: root,
		RepoFetcher:  git,
	})
	if err != nil {
		t.Fatalf("NewFilesystemWorkspaceHydrator: %v", err)
	}

	manifest := contracts.StepManifest{
		ID:    "mods-plan",
		Name:  "Mods Plan",
		Image: "ghcr.io/ploy/mods/plan:latest",
		Inputs: []contracts.StepInput{
			{
				Name:      "repo",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadOnly,
				Hydration: &contracts.StepInputHydration{
					Repo: &contracts.RepoMaterialization{
						URL:       "https://gitlab.example.com/group/project.git",
						TargetRef: "refs/heads/main",
					},
				},
			},
		},
	}

	ws, err := hydrator.Prepare(context.Background(), WorkspaceRequest{Manifest: manifest})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(ws.Inputs["repo"], "main.go"))
	if err != nil {
		t.Fatalf("read cloned file: %v", err)
	}
	if string(content) != "package main\n" {
		t.Fatalf("unexpected clone content: %q", string(content))
	}
	if git.calls == 0 {
		t.Fatal("expected git fetcher to be invoked")
	}
	if _, err := os.Stat(filepath.Join(root, "snapshots", sanitizeName("bafy-clone"))); err != nil {
		t.Fatalf("expected clone cached snapshot: %v", err)
	}
	snapshot, ok := ws.HydrationSnapshots["repo"]
	if !ok {
		t.Fatalf("expected hydration snapshot metadata")
	}
	if snapshot.CID != "bafy-clone" {
		t.Fatalf("unexpected hydration snapshot cid %q", snapshot.CID)
	}
	if snapshot.Digest != tarDigest {
		t.Fatalf("unexpected hydration snapshot digest %q", snapshot.Digest)
	}
	if snapshot.Size != info.Size() {
		t.Fatalf("unexpected hydration snapshot size %d", snapshot.Size)
	}
	if snapshot.Kind != ArtifactKindSnapshot {
		t.Fatalf("expected snapshot kind, got %s", snapshot.Kind)
	}
}

func tarBytes(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	return data, "sha256:" + hex.EncodeToString(sum[:])
}

func createTarFile(t *testing.T, dir, cid string, files map[string]string) (string, string) {
	data, digest := tarBytes(t, files)
	file := filepath.Join(dir, sanitizeName(cid)+".tar")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatalf("write tar file: %v", err)
	}
	return file, digest
}

type fakeArtifactFetcher struct {
	payloads map[string]testArtifact
	calls    map[string]int
}

type testArtifact struct {
	data   []byte
	digest string
}

func (f *fakeArtifactFetcher) Fetch(ctx context.Context, cid string) (RemoteArtifact, error) {
	_ = ctx
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls[cid]++
	payload, ok := f.payloads[cid]
	if !ok {
		return RemoteArtifact{}, fmt.Errorf("missing cid %s", cid)
	}
	return RemoteArtifact{
		CID:     cid,
		Digest:  payload.digest,
		Size:    int64(len(payload.data)),
		Content: io.NopCloser(bytes.NewReader(payload.data)),
	}, nil
}

type fakeRepoFetcher struct {
	result RepositoryFetchResult
	calls  int
}

func (f *fakeRepoFetcher) FetchRepository(ctx context.Context, req RepositoryFetchRequest) (RepositoryFetchResult, error) {
	_ = ctx
	if strings.TrimSpace(req.Repo.URL) == "" {
		return RepositoryFetchResult{}, fmt.Errorf("repo url required")
	}
	f.calls++
	return f.result, nil
}
