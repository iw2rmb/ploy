package nodeagent

import "github.com/iw2rmb/ploy/internal/workflow/step"

const notEnoughSpaceErrorText = "Not enough space"

func normalizedExecutionError(err error) string {
	if err == nil {
		return ""
	}
	if step.IsNotEnoughSpaceError(err) {
		return notEnoughSpaceErrorText
	}
	return err.Error()
}
