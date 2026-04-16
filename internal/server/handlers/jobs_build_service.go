package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const childBuildTriggerKind = "child_gate_request"

var errChildBuildNotLinked = errors.New("child build is not linked to parent")

type childBuildMeta struct {
	Kind    contracts.JobKind     `json:"kind"`
	Trigger childBuildMetaTrigger `json:"trigger"`
}

type childBuildMetaTrigger struct {
	Kind        string            `json:"kind"`
	ParentJobID domaintypes.JobID `json:"parent_job_id"`
}

func createJobBuildReGateChild(ctx context.Context, st store.Store, parentJob store.Job, buildKind string) (store.Job, error) {
	metaRaw, err := json.Marshal(childBuildMeta{
		Kind: contracts.JobKindMig,
		Trigger: childBuildMetaTrigger{
			Kind:        childBuildTriggerKind,
			ParentJobID: parentJob.ID,
		},
	})
	if err != nil {
		return store.Job{}, err
	}

	return st.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       parentJob.RunID,
		RepoID:      parentJob.RepoID,
		RepoBaseRef: parentJob.RepoBaseRef,
		Attempt:     parentJob.Attempt,
		Status:      domaintypes.JobStatusQueued,
		JobType:     domaintypes.JobTypeReGate,
		JobImage:    parentJob.JobImage,
		Meta:        metaRaw,
		RepoShaIn:   parentJob.RepoShaIn,
		Name:        buildKind + "-child",
	})
}

func getLinkedJobBuildChild(ctx context.Context, st store.Store, parentJob store.Job, childJobID domaintypes.JobID) (store.Job, error) {
	childJob, err := st.GetJob(ctx, childJobID)
	if err != nil {
		return store.Job{}, err
	}
	if domaintypes.JobType(childJob.JobType) != domaintypes.JobTypeReGate {
		return store.Job{}, errChildBuildNotLinked
	}
	if childJob.RunID != parentJob.RunID || childJob.RepoID != parentJob.RepoID || childJob.Attempt != parentJob.Attempt {
		return store.Job{}, errChildBuildNotLinked
	}
	if !isLinkedChildBuildMeta(childJob.Meta, parentJob.ID) {
		return store.Job{}, errChildBuildNotLinked
	}
	return childJob, nil
}

func writeJobBuildStatusLookupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeHTTPError(w, http.StatusNotFound, "child job not found")
	case errors.Is(err, errChildBuildNotLinked):
		writeHTTPError(w, http.StatusNotFound, "child job is not linked to parent job")
	default:
		writeHTTPError(w, http.StatusInternalServerError, "failed to get child job: %v", err)
	}
}

func isLinkedChildBuildMeta(metaRaw []byte, parentJobID domaintypes.JobID) bool {
	var meta childBuildMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return false
	}
	return meta.Trigger.Kind == childBuildTriggerKind && meta.Trigger.ParentJobID == parentJobID
}
