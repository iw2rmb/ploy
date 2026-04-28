package handlers

import (
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
			JobType: domaintypes.JobTypePreGate,
		},
		{
			ID:      domaintypes.NewJobID(),
			Status:  domaintypes.JobStatusSuccess,
			JobType: domaintypes.JobTypePreGate,
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
	if got.latestSuccessByType[domaintypes.JobTypePreGate].ID != jobs[1].ID {
		t.Fatalf("latestSuccessByType[pre_gate]=%s, want %s", got.latestSuccessByType[domaintypes.JobTypePreGate].ID, jobs[1].ID)
	}
	if got.latestSuccessMigByName["extract-usage"].ID != jobs[3].ID {
		t.Fatalf("latestSuccessMigByName[extract-usage]=%s, want %s", got.latestSuccessMigByName["extract-usage"].ID, jobs[3].ID)
	}
}

func TestResolveInFromSourceJob(t *testing.T) {
	sourcePreGate := store.Job{ID: domaintypes.NewJobID(), Status: domaintypes.JobStatusSuccess, JobType: domaintypes.JobTypePreGate}
	sourceMig := store.Job{ID: domaintypes.NewJobID(), Status: domaintypes.JobStatusSuccess, JobType: domaintypes.JobTypeMig}
	idx := inFromSourceJobIndex{
		latestSuccessByType: map[domaintypes.JobType]store.Job{
			domaintypes.JobTypePreGate: sourcePreGate,
			domaintypes.JobTypeMig:     sourceMig,
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
				SourceType: domaintypes.JobTypePreGate,
			},
			wantJobID: sourcePreGate.ID,
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
				SourceType: domaintypes.JobTypePreGate,
			},
			wantErr: true,
		},
		{
			name: "missing source by type",
			selector: contracts.InFromURI{
				SourceType: domaintypes.JobTypePostGate,
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

func TestParseMigSpecJSON_RejectsLegacySBOMInFromSelector(t *testing.T) {
	t.Parallel()

	_, err := contracts.ParseMigSpecJSON([]byte(`{
		"steps": [
			{
				"image": "ghcr.io/example/mig:latest",
				"in_from": [{"from":"sbom://out/java.classpath"}]
			}
		]
	}`))
	if err == nil {
		t.Fatal("ParseMigSpecJSON() error = nil, want non-nil")
	}
	if got := err.Error(); !strings.Contains(got, `unknown step name "sbom"`) {
		t.Fatalf("error = %q, want unknown step name rejection", got)
	}
}
