package step

import (
	"context"
	"errors"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestNewFilesystemWorkspaceHydrator(t *testing.T) {
	tests := []struct {
		name    string
		opts    FilesystemWorkspaceHydratorOptions
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid options",
			opts: FilesystemWorkspaceHydratorOptions{
				RepoFetcher: &testGitFetcher{},
			},
			wantErr: false,
		},
		{
			name:    "nil repo fetcher",
			opts:    FilesystemWorkspaceHydratorOptions{},
			wantErr: true,
			errMsg:  "repo fetcher is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFilesystemWorkspaceHydrator(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFilesystemWorkspaceHydrator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("NewFilesystemWorkspaceHydrator() error = %v, want substring %q", err, tt.errMsg)
			}
			if !tt.wantErr && got == nil {
				t.Error("NewFilesystemWorkspaceHydrator() returned nil")
			}
		})
	}
}

func TestFilesystemWorkspaceHydrator_Hydrate(t *testing.T) {
	tests := []struct {
		name      string
		manifest  contracts.StepManifest
		workspace string
		fetcher   *testGitFetcher
		wantErr   bool
		errSubstr string
	}{
		{
			name: "no inputs with repo hydration",
			manifest: contracts.StepManifest{
				Inputs: []contracts.StepInput{
					{
						Name:        "workspace",
						MountPath:   "/workspace",
						Mode:        contracts.StepInputModeReadWrite,
						SnapshotCID: types.CID("snapshot123"),
					},
				},
			},
			workspace: "/tmp/workspace",
			fetcher:   &testGitFetcher{},
			wantErr:   false,
		},
		{
			name: "input with repo hydration success",
			manifest: contracts.StepManifest{
				Inputs: []contracts.StepInput{
					{
						Name:      "workspace",
						MountPath: "/workspace",
						Mode:      contracts.StepInputModeReadWrite,
						Hydration: &contracts.StepInputHydration{
							Repo: &contracts.RepoMaterialization{
								URL:       "https://github.com/example/repo.git",
								BaseRef:   "main",
								TargetRef: "main",
							},
						},
					},
				},
			},
			workspace: "/tmp/workspace",
			fetcher: &testGitFetcher{
				fetchFn: func(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error {
					if repo.URL != "https://github.com/example/repo.git" {
						return errors.New("unexpected repo URL")
					}
					if dest != "/tmp/workspace" {
						return errors.New("unexpected destination")
					}
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "input with repo hydration failure",
			manifest: contracts.StepManifest{
				Inputs: []contracts.StepInput{
					{
						Name:      "workspace",
						MountPath: "/workspace",
						Mode:      contracts.StepInputModeReadWrite,
						Hydration: &contracts.StepInputHydration{
							Repo: &contracts.RepoMaterialization{
								URL:       "https://github.com/example/repo.git",
								BaseRef:   "main",
								TargetRef: "main",
							},
						},
					},
				},
			},
			workspace: "/tmp/workspace",
			fetcher: &testGitFetcher{
				fetchFn: func(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error {
					return errors.New("fetch failed")
				},
			},
			wantErr:   true,
			errSubstr: "failed to hydrate input workspace",
		},
		{
			name: "multiple inputs with repo hydration",
			manifest: contracts.StepManifest{
				Inputs: []contracts.StepInput{
					{
						Name:      "workspace",
						MountPath: "/workspace",
						Mode:      contracts.StepInputModeReadWrite,
						Hydration: &contracts.StepInputHydration{
							Repo: &contracts.RepoMaterialization{
								URL:       "https://github.com/example/repo1.git",
								BaseRef:   "main",
								TargetRef: "main",
							},
						},
					},
					{
						Name:        "snapshot-input",
						MountPath:   "/snapshot",
						Mode:        contracts.StepInputModeReadOnly,
						SnapshotCID: types.CID("snapshot123"),
					},
					{
						Name:      "another-repo",
						MountPath: "/repo2",
						Mode:      contracts.StepInputModeReadOnly,
						Hydration: &contracts.StepInputHydration{
							Repo: &contracts.RepoMaterialization{
								URL:       "https://github.com/example/repo2.git",
								BaseRef:   "main",
								TargetRef: "develop",
							},
						},
					},
				},
			},
			workspace: "/tmp/workspace",
			fetcher: &testGitFetcher{
				fetchFn: func(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error {
					return nil
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewFilesystemWorkspaceHydrator(FilesystemWorkspaceHydratorOptions{
				RepoFetcher: tt.fetcher,
			})
			if err != nil {
				t.Fatalf("NewFilesystemWorkspaceHydrator() error = %v", err)
			}

			ctx := context.Background()
			err = h.Hydrate(ctx, tt.manifest, tt.workspace)

			if (err != nil) != tt.wantErr {
				t.Errorf("Hydrate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Hydrate() error = %v, want substring %q", err, tt.errSubstr)
			}
		})
	}
}
