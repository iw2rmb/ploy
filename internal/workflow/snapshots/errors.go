package snapshots

import "errors"

var (
	ErrSnapshotNotFound = errors.New("snapshot not found")
	ErrInvalidSpec      = errors.New("invalid snapshot spec")
	ErrInvalidRule      = errors.New("invalid snapshot rule")
)
