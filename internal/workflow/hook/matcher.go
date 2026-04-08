package hook

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// RuntimeStack is the detected runtime stack tuple used for hook matching.
type RuntimeStack struct {
	Language string
	Tool     string
	Release  string
}

// SBOMPackage is one package tuple from an SBOM snapshot.
type SBOMPackage struct {
	Name    string
	Version string
}

// MatchInput contains runtime data required to evaluate one hook spec.
type MatchInput struct {
	Stack        RuntimeStack
	CurrentSBOM  []SBOMPackage
	PreviousSBOM []SBOMPackage
}

// SBOMPredicateDecision reports per-predicate trigger outcomes.
type SBOMPredicateDecision struct {
	OnMatch  bool
	OnAdd    bool
	OnRemove bool
	OnChange bool
}

// OnceEligibility describes whether once-by-hash state should be persisted.
type OnceEligibility struct {
	Enabled        bool
	Eligible       bool
	PersistenceKey string
}

// MatchDecision is the runtime hook execution decision.
type MatchDecision struct {
	ShouldRun    bool
	StackMatched bool
	SBOMMatched  bool
	Predicates   SBOMPredicateDecision
	HookHash     string
	Once         OnceEligibility
}

// Evaluate determines whether a hook should execute for the provided runtime input.
func Evaluate(spec Spec, input MatchInput) (MatchDecision, error) {
	return Match(spec, input)
}

// Match determines whether a hook should execute for the provided runtime input.
func Match(spec Spec, input MatchInput) (MatchDecision, error) {
	hash, err := HookHash(spec)
	if err != nil {
		return MatchDecision{}, err
	}

	stackMatched := StackMatchesFilter(spec.Stack, input.Stack)
	predicates := evaluateSBOMPredicates(spec.SBOM, input.Stack, input.CurrentSBOM, input.PreviousSBOM)
	sbomMatched := predicates.matches(spec.SBOM)
	shouldRun := stackMatched && sbomMatched

	once := OnceEligibility{
		Enabled:  spec.Once,
		Eligible: spec.Once && shouldRun,
	}
	if spec.Once {
		once.PersistenceKey = hash
	}

	return MatchDecision{
		ShouldRun:    shouldRun,
		StackMatched: stackMatched,
		SBOMMatched:  sbomMatched,
		Predicates:   predicates,
		HookHash:     hash,
		Once:         once,
	}, nil
}

// StackMatchesFilter applies workflow contract stack matching semantics.
func StackMatchesFilter(filter StackFilter, stack RuntimeStack) bool {
	return contracts.StackFieldsMatch(
		stack.Language,
		stack.Tool,
		stack.Release,
		filter.Language,
		filter.Tool,
		filter.Release,
	)
}

// DeterministicHookHash computes stable hook content hash for once-by-hash keying.
func DeterministicHookHash(spec Spec) (string, error) {
	return HookHash(spec)
}

// HookHash computes stable hook content hash for once-by-hash keying.
func HookHash(spec Spec) (string, error) {
	canonical := cloneSpec(spec)
	canonical.Source = ""
	normalizeSpec(&canonical)

	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, decoded); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

func evaluateSBOMPredicates(
	conds SBOMConditions,
	stack RuntimeStack,
	current []SBOMPackage,
	previous []SBOMPackage,
) SBOMPredicateDecision {
	cur := newSBOMIndex(current)
	prev := newSBOMIndex(previous)
	return SBOMPredicateDecision{
		OnMatch:  onMatchSatisfied(conds.OnMatch, cur, stack),
		OnAdd:    onAddSatisfied(conds.OnAdd, cur, prev, stack),
		OnRemove: onRemoveSatisfied(conds.OnRemove, cur, prev, stack),
		OnChange: onChangeSatisfied(conds.OnChange, cur, prev, stack),
	}
}

func (d SBOMPredicateDecision) matches(conds SBOMConditions) bool {
	configured := false
	if len(conds.OnMatch) > 0 {
		configured = true
		if d.OnMatch {
			return true
		}
	}
	if len(conds.OnAdd) > 0 {
		configured = true
		if d.OnAdd {
			return true
		}
	}
	if len(conds.OnRemove) > 0 {
		configured = true
		if d.OnRemove {
			return true
		}
	}
	if len(conds.OnChange) > 0 {
		configured = true
		if d.OnChange {
			return true
		}
	}
	return !configured
}

func onMatchSatisfied(conds []SBOMPackageCondition, current sbomIndex, stack RuntimeStack) bool {
	for _, cond := range conds {
		if packageConditionSatisfied(cond, current, stack) {
			return true
		}
	}
	return false
}

func onAddSatisfied(conds []SBOMPackageCondition, current, previous sbomIndex, stack RuntimeStack) bool {
	for _, cond := range conds {
		if packageConditionSatisfied(cond, current, stack) && !packageConditionSatisfied(cond, previous, stack) {
			return true
		}
	}
	return false
}

func onRemoveSatisfied(conds []SBOMPackageCondition, current, previous sbomIndex, stack RuntimeStack) bool {
	for _, cond := range conds {
		if packageConditionSatisfied(cond, previous, stack) && !packageConditionSatisfied(cond, current, stack) {
			return true
		}
	}
	return false
}

func onChangeSatisfied(conds []SBOMChangeCondition, current, previous sbomIndex, stack RuntimeStack) bool {
	for _, cond := range conds {
		if changeConditionSatisfied(cond, current, previous, stack) {
			return true
		}
	}
	return false
}

func packageConditionSatisfied(cond SBOMPackageCondition, index sbomIndex, stack RuntimeStack) bool {
	name := normalizePackageName(cond.Name)
	if name == "" {
		return false
	}
	return index.matchAny(name, cond.Version, stack.Language, stack.Tool)
}

func changeConditionSatisfied(cond SBOMChangeCondition, current, previous sbomIndex, stack RuntimeStack) bool {
	name := normalizePackageName(cond.Name)
	if name == "" {
		return false
	}

	prevVersions := previous.matchingVersions(name, cond.From, stack.Language, stack.Tool)
	curVersions := current.matchingVersions(name, cond.To, stack.Language, stack.Tool)
	if len(prevVersions) == 0 || len(curVersions) == 0 {
		return false
	}
	for _, pv := range prevVersions {
		for _, cv := range curVersions {
			if CompareVersions(stack.Language, stack.Tool, pv, cv) != 0 {
				return true
			}
		}
	}
	return false
}

type sbomIndex struct {
	versionsByName map[string][]string
}

func newSBOMIndex(pkgs []SBOMPackage) sbomIndex {
	sets := make(map[string]map[string]struct{})
	for _, pkg := range pkgs {
		name := normalizePackageName(pkg.Name)
		if name == "" {
			continue
		}
		if _, ok := sets[name]; !ok {
			sets[name] = map[string]struct{}{}
		}
		sets[name][strings.TrimSpace(pkg.Version)] = struct{}{}
	}

	versionsByName := make(map[string][]string, len(sets))
	for name, versions := range sets {
		out := make([]string, 0, len(versions))
		for v := range versions {
			out = append(out, v)
		}
		sort.Strings(out)
		versionsByName[name] = out
	}
	return sbomIndex{versionsByName: versionsByName}
}

func (i sbomIndex) matchAny(name, constraint, lang, tool string) bool {
	versions := i.versionsByName[name]
	if len(versions) == 0 {
		return false
	}
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return true
	}
	for _, version := range versions {
		if MatchVersionConstraint(lang, tool, version, constraint) {
			return true
		}
	}
	return false
}

func (i sbomIndex) matchingVersions(name, constraint, lang, tool string) []string {
	versions := i.versionsByName[name]
	if len(versions) == 0 {
		return nil
	}
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return append([]string(nil), versions...)
	}
	out := make([]string, 0, len(versions))
	for _, version := range versions {
		if MatchVersionConstraint(lang, tool, version, constraint) {
			out = append(out, version)
		}
	}
	return out
}

func normalizePackageName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func cloneSpec(s Spec) Spec {
	copySpec := Spec{
		ID:    s.ID,
		Stack: s.Stack,
		SBOM: SBOMConditions{
			OnMatch:  append([]SBOMPackageCondition(nil), s.SBOM.OnMatch...),
			OnAdd:    append([]SBOMPackageCondition(nil), s.SBOM.OnAdd...),
			OnRemove: append([]SBOMPackageCondition(nil), s.SBOM.OnRemove...),
			OnChange: append([]SBOMChangeCondition(nil), s.SBOM.OnChange...),
		},
		Once:   s.Once,
		Steps:  make([]Step, len(s.Steps)),
		Source: s.Source,
	}
	for i := range s.Steps {
		copySpec.Steps[i] = Step{
			Name:    s.Steps[i].Name,
			Image:   s.Steps[i].Image,
			Command: append([]string(nil), s.Steps[i].Command...),
			CA:      append([]string(nil), s.Steps[i].CA...),
			In:      append([]string(nil), s.Steps[i].In...),
			Out:     append([]string(nil), s.Steps[i].Out...),
			Home:    append([]string(nil), s.Steps[i].Home...),
		}
		if len(s.Steps[i].Envs) > 0 {
			copySpec.Steps[i].Envs = make(map[string]string, len(s.Steps[i].Envs))
			for k, v := range s.Steps[i].Envs {
				copySpec.Steps[i].Envs[k] = v
			}
		}
	}
	return copySpec
}

func writeCanonicalJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyJSON, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(keyJSON)
			buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
		return nil
	}
}
