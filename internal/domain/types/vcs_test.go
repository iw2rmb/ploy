package types

import (
	"errors"
	"testing"
)

func TestVCSValuesTrimInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		decode  func(string) (string, error)
		want    string
		wantErr error
	}{
		{
			name: "repository URL", input: "  https://github.com/acme/repo.git  ", want: "https://github.com/acme/repo.git",
			decode: func(input string) (string, error) {
				var value RepoURL
				err := value.UnmarshalText([]byte(input))
				return string(value), err
			},
		},
		{
			name: "SSH repository URL", input: " ssh://git@github.com/acme/repo.git ", want: "ssh://git@github.com/acme/repo.git",
			decode: func(input string) (string, error) {
				var value RepoURL
				err := value.UnmarshalText([]byte(input))
				return string(value), err
			},
		},
		{
			name: "file repository URL", input: " file:///var/tmp/repo ", want: "file:///var/tmp/repo",
			decode: func(input string) (string, error) {
				var value RepoURL
				err := value.UnmarshalText([]byte(input))
				return string(value), err
			},
		},
		{
			name: "empty repository URL", input: "   ", wantErr: ErrEmpty,
			decode: func(input string) (string, error) {
				var value RepoURL
				err := value.UnmarshalText([]byte(input))
				return string(value), err
			},
		},
		{
			name: "Git ref", input: "  main  ", want: "main",
			decode: func(input string) (string, error) {
				var value GitRef
				err := value.UnmarshalText([]byte(input))
				return string(value), err
			},
		},
		{
			name: "commit SHA", input: "  abcdef1  ", want: "abcdef1",
			decode: func(input string) (string, error) {
				var value CommitSHA
				err := value.UnmarshalText([]byte(input))
				return string(value), err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.decode(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("decode error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("decode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFullCommitSHAForms(t *testing.T) {
	lower := "0123456789abcdef0123456789abcdef01234567"
	upper := "0123456789ABCDEF0123456789ABCDEF01234567"
	tests := []struct {
		name          string
		raw           string
		want          CommitSHA
		wantNormalize bool
		wantCanonical bool
	}{
		{name: "lowercase", raw: lower, want: CommitSHA(lower), wantNormalize: true, wantCanonical: true},
		{name: "uppercase", raw: upper, want: CommitSHA(lower), wantNormalize: true},
		{name: "surrounding whitespace", raw: "  " + lower + "\n", want: CommitSHA(lower), wantNormalize: true},
		{name: "short", raw: lower[:39]},
		{name: "long", raw: lower + "0"},
		{name: "invalid hexadecimal", raw: "g" + lower[1:]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeFullCommitSHA(tt.raw)
			if ok != tt.wantNormalize || got != tt.want {
				t.Fatalf("NormalizeFullCommitSHA(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.want, tt.wantNormalize)
			}
			if got := IsCanonicalFullCommitSHA(tt.raw); got != tt.wantCanonical {
				t.Fatalf("IsCanonicalFullCommitSHA(%q) = %v, want %v", tt.raw, got, tt.wantCanonical)
			}
		})
	}
}

func TestRepositoryURLNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           string
		want            string
		wantSchemless   string
		checkIdempotent bool
	}{
		{name: "trailing Git suffix", input: "https://github.com/org/repo.git", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "trailing slash", input: "https://github.com/org/repo/", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "slash after Git suffix", input: "https://github.com/org/repo.git/", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "surrounding spaces", input: "  https://github.com/org/repo  ", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "surrounding tabs", input: "\thttps://github.com/org/repo\t", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "spaces and Git suffix", input: "  https://github.com/org/repo.git  ", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "spaces slash and Git suffix", input: "  https://github.com/org/repo.git/  ", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "SCP URL with Git suffix", input: "git@github.com:org/repo.git", want: "git@github.com:org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "SCP URL", input: "git@github.com:org/repo", want: "git@github.com:org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "SSH scheme strips user", input: "ssh://git@github.com/org/repo.git", want: "ssh://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "HTTPS", input: "https://github.com/org/repo", want: "https://github.com/org/repo", wantSchemless: "github.com/org/repo", checkIdempotent: true},
		{name: "HTTPS port", input: "https://github.com:443/org/repo.git", want: "https://github.com:443/org/repo", wantSchemless: "github.com:443/org/repo", checkIdempotent: true},
		{name: "HTTPS strips credentials", input: "https://oauth2:token@gitlab.example.com/group/repo.git", want: "https://gitlab.example.com/group/repo", wantSchemless: "gitlab.example.com/group/repo", checkIdempotent: true},
		{name: "file scheme with Git suffix", input: "file:///path/to/repo.git", want: "file:///path/to/repo", wantSchemless: "/path/to/repo", checkIdempotent: true},
		{name: "file scheme with slash", input: "file:///path/to/repo/", want: "file:///path/to/repo", wantSchemless: "/path/to/repo", checkIdempotent: true},
		{name: "empty", input: "", want: "", wantSchemless: "", checkIdempotent: true},
		{name: "whitespace only", input: "   ", want: "", wantSchemless: "", checkIdempotent: true},
		{name: "Git text in middle", input: "https://github.com/org/.git-templates/repo", want: "https://github.com/org/.git-templates/repo", wantSchemless: "github.com/org/.git-templates/repo", checkIdempotent: true},
		{name: "multiple trailing slashes", input: "https://github.com/org/repo//", want: "https://github.com/org/repo/", wantSchemless: "github.com/org/repo/"},
		{name: "nested path", input: "https://github.com/org/team/subteam/repo.git", want: "https://github.com/org/team/subteam/repo", wantSchemless: "github.com/org/team/subteam/repo", checkIdempotent: true},
		{name: "home file URL", input: "file:///home/user/repo.git/", want: "file:///home/user/repo", wantSchemless: "/home/user/repo", checkIdempotent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeRepoURL(tt.input); got != tt.want {
				t.Errorf("NormalizeRepoURL() = %q, want %q", got, tt.want)
			}
			if got := NormalizeRepoURLSchemless(tt.input); got != tt.wantSchemless {
				t.Errorf("NormalizeRepoURLSchemless() = %q, want %q", got, tt.wantSchemless)
			}
			if tt.checkIdempotent {
				if got := NormalizeRepoURL(NormalizeRepoURL(tt.input)); got != tt.want {
					t.Errorf("second NormalizeRepoURL() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}
