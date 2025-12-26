package types

// DiffSummary represents summary metadata attached to a diff.
//
// It remains map-based to preserve flexibility while providing helpers for
// commonly inspected fields.
type DiffSummary map[string]any

// ExitCode returns the exit_code field as an int when present.
// Delegates to IntFromAny for consistent JSON number coercion.
func (d DiffSummary) ExitCode() (int, bool) {
	v, ok := d["exit_code"]
	if !ok {
		return 0, false
	}
	return IntFromAny(v)
}

// FilesChanged returns the files_changed field as an int when present.
// Delegates to IntFromAny for consistent JSON number coercion.
func (d DiffSummary) FilesChanged() (int, bool) {
	v, ok := d["files_changed"]
	if !ok {
		return 0, false
	}
	return IntFromAny(v)
}
