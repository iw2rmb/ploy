package types

import (
	"fmt"
	"strings"
)

type RunRepoActionType string

func (v RunRepoActionType) String() string { return string(v) }

func (v RunRepoActionType) IsZero() bool { return IsEmpty(string(v)) }

func (v RunRepoActionType) Validate() error {
	s := strings.TrimSpace(string(v))
	if s == "" {
		return fmt.Errorf("invalid action_type %q", s)
	}
	return nil
}

const (
	NodeActionCleanupDisk   = "node.cleanup_disk"
	NodeActionUpdateUpdater = "node.update_updater"
)

func IsNodeActionType(actionType string) bool {
	switch strings.TrimSpace(actionType) {
	case NodeActionCleanupDisk, NodeActionUpdateUpdater:
		return true
	default:
		return false
	}
}
