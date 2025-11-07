package types

import (
	"encoding"
	"errors"
	"strings"
)

// RepoURL is a version control repository URL.
//
// It trims surrounding spaces on decode and marshals as a JSON string.
// Allowed schemes are https, ssh, and file. Values must be non-empty.
type RepoURL string

// GitRef is a Git reference such as a branch or tag name.
//
// It trims surrounding spaces on decode and marshals as a JSON string.
// Values must be non-empty.
type GitRef string

// CommitSHA is a Git commit identifier.
//
// It trims surrounding spaces on decode and marshals as a JSON string.
// Values must be non-empty.
type CommitSHA string

// String returns the underlying string value.
func (v RepoURL) String() string { return string(v) }

// String returns the underlying string value.
func (v GitRef) String() string { return string(v) }

// String returns the underlying string value.
func (v CommitSHA) String() string { return string(v) }

// ErrInvalidRepoURL indicates the value is not an accepted repo URL.
var ErrInvalidRepoURL = errors.New("invalid repo url")

// allowedRepoURL reports whether s begins with an accepted scheme.
func allowedRepoURL(s string) bool {
	s = strings.ToLower(s)
	return strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasPrefix(s, "file://")
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*RepoURL)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v RepoURL) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	if !allowedRepoURL(s) {
		return nil, ErrInvalidRepoURL
	}
	return []byte(s), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *RepoURL) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if !allowedRepoURL(s) {
		return ErrInvalidRepoURL
	}
	*v = RepoURL(s)
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (v RepoURL) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *RepoURL) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Validate implements Validatable.
func (v RepoURL) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	if !allowedRepoURL(string(v)) {
		return ErrInvalidRepoURL
	}
	return nil
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*GitRef)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v GitRef) MarshalText() ([]byte, error) { return marshalIDText(v) }

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *GitRef) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }

// MarshalJSON encodes the value as a JSON string.
func (v GitRef) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *GitRef) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Validate implements Validatable.
func (v GitRef) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*CommitSHA)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v CommitSHA) MarshalText() ([]byte, error) { return marshalIDText(v) }

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *CommitSHA) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }

// MarshalJSON encodes the value as a JSON string.
func (v CommitSHA) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *CommitSHA) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Validate implements Validatable.
func (v CommitSHA) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}
