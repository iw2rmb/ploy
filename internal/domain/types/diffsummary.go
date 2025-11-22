package types

// DiffSummary represents summary metadata attached to a diff.
//
// It remains map-based to preserve flexibility while providing helpers for
// commonly inspected fields.
type DiffSummary map[string]any

// ExitCode returns the exit_code field as an int when present.
func (d DiffSummary) ExitCode() (int, bool) {
	v, ok := d["exit_code"]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// FilesChanged returns the files_changed field as an int when present.
func (d DiffSummary) FilesChanged() (int, bool) {
	v, ok := d["files_changed"]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
