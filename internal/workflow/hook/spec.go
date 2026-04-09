package hook

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

// Spec defines a single hook manifest.
type Spec struct {
	ID    string         `json:"id" yaml:"id"`
	Stack StackFilter    `json:"stack,omitempty" yaml:"stack,omitempty"`
	SBOM  SBOMConditions `json:"sbom,omitempty" yaml:"sbom,omitempty"`
	Once  bool           `json:"once,omitempty" yaml:"once,omitempty"`
	Steps []Step         `json:"steps" yaml:"steps"`

	// Source is populated by the loader and tracks where this manifest was read from.
	Source string `json:"-" yaml:"-"`
}

// StackFilter limits hook execution to a detected stack tuple.
type StackFilter struct {
	Language string `json:"language,omitempty" yaml:"language,omitempty"`
	Tool     string `json:"tool,omitempty" yaml:"tool,omitempty"`
	Release  string `json:"release,omitempty" yaml:"release,omitempty"`
}

// SBOMConditions defines package predicates used by runtime matching.
type SBOMConditions struct {
	OnMatch  []SBOMPackageCondition `json:"on_match,omitempty" yaml:"on_match,omitempty"`
	OnAdd    []SBOMPackageCondition `json:"on_add,omitempty" yaml:"on_add,omitempty"`
	OnRemove []SBOMPackageCondition `json:"on_remove,omitempty" yaml:"on_remove,omitempty"`
	OnChange []SBOMChangeCondition  `json:"on_change,omitempty" yaml:"on_change,omitempty"`
}

// SBOMPackageCondition matches a package identity in an SBOM snapshot.
type SBOMPackageCondition struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// SBOMChangeCondition matches package version transitions across snapshots.
type SBOMChangeCondition struct {
	Name string `json:"name" yaml:"name"`
	From string `json:"from,omitempty" yaml:"from,omitempty"`
	To   string `json:"to,omitempty" yaml:"to,omitempty"`
}

// Step defines one hook runtime step.
type Step struct {
	Name    string            `json:"name,omitempty" yaml:"name,omitempty"`
	Image   string            `json:"image" yaml:"image"`
	Command []string          `json:"command,omitempty" yaml:"command,omitempty"`
	Envs    map[string]string `json:"envs,omitempty" yaml:"envs,omitempty"`
	CA      []string          `json:"ca,omitempty" yaml:"ca,omitempty"`
	In      []string          `json:"in,omitempty" yaml:"in,omitempty"`
	Out     []string          `json:"out,omitempty" yaml:"out,omitempty"`
	Home    []string          `json:"home,omitempty" yaml:"home,omitempty"`
}

// ToJobImage converts the hook step image to the shared runtime image contract.
func (s Step) ToJobImage() contracts.JobImage {
	return contracts.JobImage{Universal: strings.TrimSpace(s.Image)}
}

// ToCommandSpec converts the hook step command to the shared runtime command contract.
func (s Step) ToCommandSpec() contracts.CommandSpec {
	if len(s.Command) == 0 {
		return contracts.CommandSpec{}
	}
	return contracts.CommandSpec{Exec: append([]string(nil), s.Command...)}
}

// LoadSpecYAML decodes a hook spec with strict unknown-field rejection.
func LoadSpecYAML(data []byte, source string) (Spec, error) {
	if strings.TrimSpace(source) == "" {
		source = "<inline>"
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var spec Spec
	if err := dec.Decode(&spec); err != nil {
		return Spec{}, fmt.Errorf("decode hook spec %s: %w", source, err)
	}

	var extra any
	switch err := dec.Decode(&extra); {
	case err == io.EOF:
	case err != nil:
		return Spec{}, fmt.Errorf("decode hook spec %s: %w", source, err)
	default:
		return Spec{}, fmt.Errorf("decode hook spec %s: multiple YAML documents are not supported", source)
	}

	normalizeSpec(&spec)
	if err := spec.Validate(); err != nil {
		return Spec{}, fmt.Errorf("validate hook spec %s: %w", source, err)
	}
	spec.Source = source
	return spec, nil
}

// Validate checks structural hook manifest correctness.
func (s Spec) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("id: required")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("steps: required")
	}
	for i, step := range s.Steps {
		if step.Image == "" {
			return fmt.Errorf("steps[%d].image: required", i)
		}
		for j, arg := range step.Command {
			if strings.TrimSpace(arg) == "" {
				return fmt.Errorf("steps[%d].command[%d]: required", i, j)
			}
		}
		for key := range step.Envs {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("steps[%d].envs: empty key", i)
			}
		}
		if err := contracts.ValidateHydraCAEntries(step.CA, fmt.Sprintf("steps[%d].ca", i)); err != nil {
			return err
		}
		if err := contracts.ValidateHydraInEntries(step.In, fmt.Sprintf("steps[%d].in", i)); err != nil {
			return err
		}
		if err := contracts.ValidateHydraOutEntries(step.Out, fmt.Sprintf("steps[%d].out", i)); err != nil {
			return err
		}
		if err := contracts.ValidateHydraHomeEntries(step.Home, fmt.Sprintf("steps[%d].home", i)); err != nil {
			return err
		}
	}
	if err := validatePackageConditions("sbom.on_match", s.SBOM.OnMatch); err != nil {
		return err
	}
	if err := validatePackageConditions("sbom.on_add", s.SBOM.OnAdd); err != nil {
		return err
	}
	if err := validatePackageConditions("sbom.on_remove", s.SBOM.OnRemove); err != nil {
		return err
	}
	if err := validateChangeConditions("sbom.on_change", s.SBOM.OnChange); err != nil {
		return err
	}
	return nil
}

func validatePackageConditions(path string, list []SBOMPackageCondition) error {
	for i, cond := range list {
		if cond.Name == "" {
			return fmt.Errorf("%s[%d].name: required", path, i)
		}
	}
	return nil
}

func validateChangeConditions(path string, list []SBOMChangeCondition) error {
	for i, cond := range list {
		if cond.Name == "" {
			return fmt.Errorf("%s[%d].name: required", path, i)
		}
	}
	return nil
}

func normalizeSpec(spec *Spec) {
	spec.ID = strings.TrimSpace(spec.ID)
	spec.Stack.Language = strings.TrimSpace(spec.Stack.Language)
	spec.Stack.Tool = strings.TrimSpace(spec.Stack.Tool)
	spec.Stack.Release = strings.TrimSpace(spec.Stack.Release)

	for i := range spec.Steps {
		spec.Steps[i].Name = strings.TrimSpace(spec.Steps[i].Name)
		spec.Steps[i].Image = strings.TrimSpace(spec.Steps[i].Image)
		for j := range spec.Steps[i].Command {
			spec.Steps[i].Command[j] = strings.TrimSpace(spec.Steps[i].Command[j])
		}
		if len(spec.Steps[i].Envs) > 0 {
			normalized := make(map[string]string, len(spec.Steps[i].Envs))
			for key, value := range spec.Steps[i].Envs {
				normalized[strings.TrimSpace(key)] = value
			}
			spec.Steps[i].Envs = normalized
		}
		spec.Steps[i].CA = normalizeTrimmedStrings(spec.Steps[i].CA)
		spec.Steps[i].In = normalizeTrimmedStrings(spec.Steps[i].In)
		spec.Steps[i].Out = normalizeTrimmedStrings(spec.Steps[i].Out)
		spec.Steps[i].Home = normalizeTrimmedStrings(spec.Steps[i].Home)
	}

	for i := range spec.SBOM.OnMatch {
		normalizePackageCondition(&spec.SBOM.OnMatch[i])
	}
	for i := range spec.SBOM.OnAdd {
		normalizePackageCondition(&spec.SBOM.OnAdd[i])
	}
	for i := range spec.SBOM.OnRemove {
		normalizePackageCondition(&spec.SBOM.OnRemove[i])
	}
	for i := range spec.SBOM.OnChange {
		spec.SBOM.OnChange[i].Name = strings.TrimSpace(spec.SBOM.OnChange[i].Name)
		spec.SBOM.OnChange[i].From = strings.TrimSpace(spec.SBOM.OnChange[i].From)
		spec.SBOM.OnChange[i].To = strings.TrimSpace(spec.SBOM.OnChange[i].To)
	}
}

func normalizePackageCondition(cond *SBOMPackageCondition) {
	cond.Name = strings.TrimSpace(cond.Name)
	cond.Version = strings.TrimSpace(cond.Version)
}

func normalizeTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}
