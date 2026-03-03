package contracts

import (
	"fmt"
	"regexp"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

var (
	sha40Pattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ModStepSkipMetadata records claim-time step cache reuse decisions.
// When present on a claimed mig job, node runtime must skip container execution
// and finish the job by reporting the provided ref_repo_sha_out.
type ModStepSkipMetadata struct {
	Enabled       bool        `json:"enabled"`
	RefJobID      types.JobID `json:"ref_job_id,omitempty"`
	RefRepoSHAOut string      `json:"ref_repo_sha_out,omitempty"`
	Hash          string      `json:"hash,omitempty"`
}

func (m *ModStepSkipMetadata) Validate() error {
	if m == nil {
		return nil
	}
	if !m.Enabled {
		return fmt.Errorf("enabled: must be true when step skip metadata is present")
	}
	if m.RefJobID.IsZero() {
		return fmt.Errorf("ref_job_id: required")
	}
	if !sha40Pattern.MatchString(strings.TrimSpace(m.RefRepoSHAOut)) {
		return fmt.Errorf("ref_repo_sha_out: must match ^[0-9a-f]{40}$")
	}
	if m.Hash != "" && !sha256HexPattern.MatchString(strings.TrimSpace(m.Hash)) {
		return fmt.Errorf("hash: must match ^[0-9a-f]{64}$")
	}
	return nil
}
