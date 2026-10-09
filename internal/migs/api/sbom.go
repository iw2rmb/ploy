package api

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

type RunSBOMPackage struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

type JobSBOMUploadRequest struct {
	Packages []RunSBOMPackage `json:"packages"`
}

type JobSBOMUploadResponse struct {
	JobID    domaintypes.JobID `json:"job_id"`
	RowCount int               `json:"row_count"`
}

type RunSBOMPackagesResponse struct {
	RunID    domaintypes.RunID `json:"run_id"`
	View     string            `json:"view"`
	Packages []RunSBOMPackage  `json:"packages"`
}

type RunSBOMDiffPackage struct {
	Package     string `json:"package"`
	VersionPre  string `json:"version_pre"`
	VersionPost string `json:"version_post"`
	Change      string `json:"change"`
}

type RunSBOMDiffResponse struct {
	RunID    domaintypes.RunID    `json:"run_id"`
	View     string               `json:"view"`
	Packages []RunSBOMDiffPackage `json:"packages"`
}

type JobSBOMDiffResponse struct {
	JobID         domaintypes.JobID    `json:"job_id"`
	BaselineJobID domaintypes.JobID    `json:"baseline_job_id"`
	Packages      []RunSBOMDiffPackage `json:"packages"`
}

// MaxGateOutcomeBytes bounds decompressed report downloads and node publication.
const MaxGateOutcomeBytes = 64 << 20
