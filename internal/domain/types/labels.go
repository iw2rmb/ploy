package types

// LabelRunID is the container label key storing the run identifier.
const LabelRunID = "com.ploy.run_id"

// LabelJobID is the container label key storing the job identifier.
const LabelJobID = "com.ploy.job_id"

// LabelsForRun returns a labels map containing the run identifier.
// When id is empty, it returns nil.
func LabelsForRun(id RunID) map[string]string {
	if id.IsZero() {
		return nil
	}
	return map[string]string{LabelRunID: id.String()}
}

// LabelsForStep returns a labels map containing the step identifier.
// The value is placed under LabelJobID for downstream correlation.
// When id is empty, it returns nil.
func LabelsForStep(id StepID) map[string]string {
	if id.IsZero() {
		return nil
	}
	return map[string]string{LabelJobID: id.String()}
}
