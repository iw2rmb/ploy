package handlers

import (
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func mergeCompletionJobMeta(incomingRaw []byte) ([]byte, error) {
	if _, err := contracts.UnmarshalJobMeta(incomingRaw); err != nil {
		return nil, err
	}
	return incomingRaw, nil
}
