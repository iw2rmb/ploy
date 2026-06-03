package nodeagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestMaterializeMigInFromInputs(t *testing.T) {
	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	sourceJobID := types.NewJobID()
	currentJobID := types.NewJobID()

	tests := []struct {
		name          string
		ref           contracts.ResolvedInFromRef
		createSource  bool
		wantErr       string
		wantTargetRel string
	}{
		{
			name: "copies source out file into current in dir",
			ref: contracts.ResolvedInFromRef{
				From:          "extract@mig://out/dependency-usage.json",
				To:            "/in/dependency-usage.json",
				SourceJobID:   sourceJobID,
				SourceOutPath: "/out/dependency-usage.json",
			},
			createSource:  true,
			wantTargetRel: "dependency-usage.json",
		},
		{
			name: "defaults target basename",
			ref: contracts.ResolvedInFromRef{
				From:          "extract@mig://out/nested/result.json",
				SourceJobID:   sourceJobID,
				SourceOutPath: "/out/nested/result.json",
			},
			createSource:  true,
			wantTargetRel: "result.json",
		},
		{
			name: "rejects source traversal",
			ref: contracts.ResolvedInFromRef{
				From:          "extract@mig://out/ok.json",
				To:            "/in/ok.json",
				SourceJobID:   sourceJobID,
				SourceOutPath: "/out/../secret.json",
			},
			wantErr: "source path must stay under /out",
		},
		{
			name: "fails when source output is missing",
			ref: contracts.ResolvedInFromRef{
				From:          "extract@mig://out/missing.json",
				To:            "/in/missing.json",
				SourceJobID:   sourceJobID,
				SourceOutPath: "/out/missing.json",
			},
			wantErr: "no such file",
		},
		{
			name: "requires source job id",
			ref: contracts.ResolvedInFromRef{
				From:          "extract@mig://out/result.json",
				To:            "/in/result.json",
				SourceOutPath: "/out/result.json",
			},
			wantErr: "source_job_id: required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheHome := t.TempDir()
			t.Setenv("PLOYD_CACHE_HOME", cacheHome)

			if tt.createSource {
				sourcePath, err := jobOutFile(runID, sourceJobID, tt.ref.SourceOutPath)
				if err != nil {
					t.Fatalf("jobOutFile() error = %v", err)
				}
				if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
					t.Fatalf("mkdir source parent: %v", err)
				}
				if err := os.WriteFile(sourcePath, []byte("payload"), 0o644); err != nil {
					t.Fatalf("write source: %v", err)
				}
			}

			inDir := artifactPaths(runID, currentJobID).In
			rc := &runController{}
			err := rc.materializeInFrom(context.Background(), StartRunRequest{
				RunID:   runID,
				RepoID:  repoID,
				JobID:   currentJobID,
				JobType: types.JobTypeMig,
				MigContext: &contracts.MigClaimContext{
					StepIndex: 1,
					InFrom:    []contracts.ResolvedInFromRef{tt.ref},
				},
			}, inDir)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("materializeInFrom() error = %v", err)
			}

			target := filepath.Join(inDir, tt.wantTargetRel)
			info, err := os.Lstat(target)
			if err != nil {
				t.Fatalf("Lstat(%s) error = %v", target, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("target is symlink, want regular file")
			}
			if !info.Mode().IsRegular() {
				t.Fatalf("target mode = %v, want regular file", info.Mode())
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", target, err)
			}
			if string(got) != "payload" {
				t.Fatalf("target content = %q, want payload", string(got))
			}
		})
	}
}
