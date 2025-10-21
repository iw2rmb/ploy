package contracts

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	stepIDPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{2,63}$`)
	imagePattern    = regexp.MustCompile(`^[\w][\w./:@+-]{2,}$`)
	envKeyPattern   = regexp.MustCompile(`^[A-Z0-9_]+$`)
	stepInputNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{2,63}$`)
)

// StepManifest defines the execution contract for a single Mod step.
type StepManifest struct {
	ID         string
	Name       string
	Image      string
	Command    []string
	Args       []string
	WorkingDir string
	Env        map[string]string
	Inputs     []StepInput
	Outputs    []StepOutput
	Artifacts  []StepArtifact
	Shift      *StepShiftSpec
	Resources  StepResourceSpec
	Retention  StepRetentionSpec
}

// StepInputMode describes how the input is mounted into the container.
type StepInputMode string

const (
	// StepInputModeReadOnly mounts the input read-only.
	StepInputModeReadOnly StepInputMode = "ro"
	// StepInputModeReadWrite mounts the input read-write.
	StepInputModeReadWrite StepInputMode = "rw"
)

// StepInput describes repository state presented to the container.
type StepInput struct {
	Name        string
	MountPath   string
	Mode        StepInputMode
	SnapshotCID string
	DiffCID     string
}

// StepOutput describes expected paths produced by the container.
type StepOutput struct {
	Name string
	Path string
	Type string
}

// StepArtifact describes an artifact emitted after execution.
type StepArtifact struct {
	Name string
	Type string
}

// StepShiftSpec configures SHIFT validation post step execution.
type StepShiftSpec struct {
	Enabled bool
	Profile string
	Env     map[string]string
}

// StepResourceSpec captures runtime resource hints.
type StepResourceSpec struct {
	CPU    string
	Memory string
	Disk   string
	GPU    string
}

// StepRetentionSpec controls container and workspace retention.
type StepRetentionSpec struct {
	RetainContainer bool
	TTL             string
}

// Validate ensures the manifest is well-formed.
func (m StepManifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return errors.New("step manifest id required")
	}
	if !stepIDPattern.MatchString(m.ID) {
		return fmt.Errorf("step manifest id invalid: %q", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("step manifest name required")
	}
	if strings.TrimSpace(m.Image) == "" {
		return errors.New("step manifest image required")
	}
	if !imagePattern.MatchString(m.Image) {
		return fmt.Errorf("step manifest image invalid: %q", m.Image)
	}
	if m.WorkingDir != "" && !filepath.IsAbs(m.WorkingDir) {
		return fmt.Errorf("step manifest working dir must be absolute: %q", m.WorkingDir)
	}
	if len(m.Env) > 0 {
		keys := make([]string, 0, len(m.Env))
		for key := range m.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if strings.TrimSpace(key) == "" {
				return errors.New("step manifest environment key required")
			}
			if !envKeyPattern.MatchString(key) {
				return fmt.Errorf("step manifest environment key invalid: %q", key)
			}
		}
	}
	if len(m.Inputs) == 0 {
		return errors.New("step manifest inputs required")
	}
	seenInputs := make(map[string]struct{}, len(m.Inputs))
	for idx, input := range m.Inputs {
		position := fmt.Sprintf("inputs[%d]", idx)
		if strings.TrimSpace(input.Name) == "" {
			return fmt.Errorf("%s name required", position)
		}
		if !stepInputNameRe.MatchString(input.Name) {
			return fmt.Errorf("%s name invalid: %q", position, input.Name)
		}
		if _, exists := seenInputs[input.Name]; exists {
			return fmt.Errorf("%s duplicate name %q", position, input.Name)
		}
		seenInputs[input.Name] = struct{}{}
		if strings.TrimSpace(input.MountPath) == "" {
			return fmt.Errorf("%s mount path required", position)
		}
		if !filepath.IsAbs(input.MountPath) {
			return fmt.Errorf("%s mount path must be absolute: %q", position, input.MountPath)
		}
		switch input.Mode {
		case StepInputModeReadOnly, StepInputModeReadWrite:
			// ok
		default:
			return fmt.Errorf("%s mount mode invalid: %q", position, input.Mode)
		}
		hasSnapshot := strings.TrimSpace(input.SnapshotCID) != ""
		hasDiff := strings.TrimSpace(input.DiffCID) != ""
		if hasSnapshot == hasDiff {
			return fmt.Errorf("%s must reference exactly one source (snapshot or diff)", position)
		}
	}
	if m.Shift != nil {
		if m.Shift.Enabled || strings.TrimSpace(m.Shift.Profile) != "" {
			if strings.TrimSpace(m.Shift.Profile) == "" {
				return errors.New("shift profile required when enabled")
			}
			if len(m.Shift.Env) > 0 {
				for key := range m.Shift.Env {
					if !envKeyPattern.MatchString(key) {
						return fmt.Errorf("shift environment key invalid: %q", key)
					}
				}
			}
		}
	}
	if m.Retention.RetainContainer {
		if strings.TrimSpace(m.Retention.TTL) == "" {
			return errors.New("retention ttl required when retaining container")
		}
		if _, err := time.ParseDuration(strings.TrimSpace(m.Retention.TTL)); err != nil {
			return fmt.Errorf("retention ttl invalid: %w", err)
		}
	} else if strings.TrimSpace(m.Retention.TTL) != "" {
		if _, err := time.ParseDuration(strings.TrimSpace(m.Retention.TTL)); err != nil {
			return fmt.Errorf("retention ttl invalid: %w", err)
		}
	}

	return nil
}
