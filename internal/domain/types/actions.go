package types

import (
	"fmt"
	"strings"
)

type RunRepoActionType string

const (
	RunRepoActionTypeMRCreate RunRepoActionType = "mr_create"
)

func (v RunRepoActionType) String() string { return string(v) }

func (v RunRepoActionType) IsZero() bool { return IsEmpty(string(v)) }

func (v RunRepoActionType) Validate() error {
	s := strings.TrimSpace(string(v))
	switch RunRepoActionType(s) {
	case RunRepoActionTypeMRCreate:
		return nil
	default:
		return fmt.Errorf("invalid action_type %q", s)
	}
}
