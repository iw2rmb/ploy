package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildInFromSourceJobIndex_PrefersLatestSuccessfulByTypeAndMigName(t *testing.T) {
	metaStep, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind:        contracts.JobKindMig,
		MigStepName: "extract-usage",
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}

	jobs := []store.Job{
		{
			ID:      domaintypes.NewJobID(),
			Status:  domaintypes.JobStatusError,
			JobType: domaintypes.JobTypeSBOM,
		},
		{
			ID:      domaintypes.NewJobID(),
			Status:  domaintypes.JobStatusSuccess,
			JobType: domaintypes.JobTypeSBOM,
		},
		{
			ID:      domaintypes.NewJobID(),
			Status:  domaintypes.JobStatusSuccess,
			JobType: domaintypes.JobTypeMig,
			Meta:    metaStep,
		},
		{
			ID:      domaintypes.NewJobID(),
			Status:  domaintypes.JobStatusSuccess,
			JobType: domaintypes.JobTypeMig,
			Meta:    metaStep,
		},
	}

	got := buildInFromSourceJobIndex(jobs)
	if len(got.latestSuccessByType) != 2 {
		t.Fatalf("len(latestSuccessByType)=%d, want 2", len(got.latestSuccessByType))
	}
	if got.latestSuccessByType[domaintypes.JobTypeSBOM].ID != jobs[1].ID {
		t.Fatalf("latestSuccessByType[sbom]=%s, want %s", got.latestSuccessByType[domaintypes.JobTypeSBOM].ID, jobs[1].ID)
	}
	if got.latestSuccessMigByName["extract-usage"].ID != jobs[3].ID {
		t.Fatalf("latestSuccessMigByName[extract-usage]=%s, want %s", got.latestSuccessMigByName["extract-usage"].ID, jobs[3].ID)
	}
}

func TestResolveInFromSourceJob(t *testing.T) {
	sourceSBOM := store.Job{ID: domaintypes.NewJobID(), Status: domaintypes.JobStatusSuccess, JobType: domaintypes.JobTypeSBOM}
	sourceMig := store.Job{ID: domaintypes.NewJobID(), Status: domaintypes.JobStatusSuccess, JobType: domaintypes.JobTypeMig}
	idx := inFromSourceJobIndex{
		latestSuccessByType: map[domaintypes.JobType]store.Job{
			domaintypes.JobTypeSBOM: sourceSBOM,
			domaintypes.JobTypeMig:  sourceMig,
		},
		latestSuccessMigByName: map[string]store.Job{
			"extract-usage": sourceMig,
		},
	}

	tests := []struct {
		name      string
		selector  contracts.InFromURI
		wantJobID domaintypes.JobID
		wantErr   bool
	}{
		{
			name: "type selector",
			selector: contracts.InFromURI{
				SourceType: domaintypes.JobTypeSBOM,
			},
			wantJobID: sourceSBOM.ID,
		},
		{
			name: "named mig selector",
			selector: contracts.InFromURI{
				SourceName: "extract-usage",
				SourceType: domaintypes.JobTypeMig,
			},
			wantJobID: sourceMig.ID,
		},
		{
			name: "named non-mig selector rejected",
			selector: contracts.InFromURI{
				SourceName: "pre",
				SourceType: domaintypes.JobTypeSBOM,
			},
			wantErr: true,
		},
		{
			name: "missing source by type",
			selector: contracts.InFromURI{
				SourceType: domaintypes.JobTypePreGate,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInFromSourceJob(tt.selector, idx)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveInFromSourceJob() error = %v", err)
			}
			if got.ID != tt.wantJobID {
				t.Fatalf("job id = %s, want %s", got.ID, tt.wantJobID)
			}
		})
	}
}

func TestSelectPreferredArtifactID(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	nameOther := "other"
	nameMigOut := "mig-out"
	bundles := []store.ArtifactBundle{
		{
			ID:   pgtype.UUID{Bytes: id1, Valid: true},
			Name: &nameOther,
		},
		{
			ID:   pgtype.UUID{Bytes: id2, Valid: true},
			Name: &nameMigOut,
		},
	}

	got := selectPreferredArtifactID(bundles, []string{"mig-out", ""})
	if got != id2.String() {
		t.Fatalf("artifact id=%q, want %q", got, id2.String())
	}
}

func TestResolveMigInFromClaimEntries_RejectsSBOMJavaClasspathInFrom(t *testing.T) {
	t.Parallel()

	spec, err := contracts.ParseMigSpecJSON([]byte(`{
		"steps": [
			{
				"image": "ghcr.io/example/mig:latest",
				"in_from": [{"from":"sbom://out/java.classpath"}]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON() error = %v", err)
	}

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      domaintypes.NewJobID(),
			RunID:   domaintypes.NewRunID(),
			RepoID:  domaintypes.NewRepoID(),
			Attempt: 1,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
		},
	}
	job := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   st.listJobsByRunRepoAttempt.val[0].RunID,
		RepoID:  st.listJobsByRunRepoAttempt.val[0].RepoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeMig,
	}

	_, err = resolveMigInFromClaimEntries(context.Background(), st, job, spec, 0)
	if err == nil {
		t.Fatal("resolveMigInFromClaimEntries() error = nil, want non-nil")
	}
	if got := err.Error(); !strings.Contains(got, "sbom://out/java.classpath is unsupported") {
		t.Fatalf("error = %q, want sbom java.classpath rejection", got)
	}
}
