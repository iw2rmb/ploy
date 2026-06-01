package types

import (
	"fmt"
	"strings"
)

type RunActionType string

func (v RunActionType) String() string { return string(v) }

func (v RunActionType) IsZero() bool { return IsEmpty(string(v)) }

func (v RunActionType) Validate() error {
	s := strings.TrimSpace(string(v))
	if s == "" {
		return fmt.Errorf("invalid action_type %q", s)
	}
	return nil
}
