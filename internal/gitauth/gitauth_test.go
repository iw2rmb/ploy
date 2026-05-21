package gitauth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPrepareURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		opts     Options
		wantURL  string
		wantAuth string
	}{
		{
			name:     "strips explicit credentials into auth env",
			rawURL:   "https://user:pass@gitlab.example.com/group/repo.git",
			wantURL:  "https://gitlab.example.com/group/repo.git",
			wantAuth: "user:pass",
		},
		{
			name:     "uses gitlab pat for matching host",
			rawURL:   "https://gitlab.example.com/group/repo.git",
			opts:     Options{GitLabPAT: "glpat-secret", GitLabDomain: "https://gitlab.example.com"},
			wantURL:  "https://gitlab.example.com/group/repo.git",
			wantAuth: "oauth2:glpat-secret",
		},
		{
			name:    "does not use gitlab pat for non-matching host",
			rawURL:  "https://github.com/group/repo.git",
			opts:    Options{GitLabPAT: "glpat-secret", GitLabDomain: "gitlab.example.com"},
			wantURL: "https://github.com/group/repo.git",
		},
		{
			name:    "does not auth non-http urls",
			rawURL:  "ssh://git@gitlab.example.com/group/repo.git",
			opts:    Options{GitLabPAT: "glpat-secret", GitLabDomain: "gitlab.example.com"},
			wantURL: "ssh://git@gitlab.example.com/group/repo.git",
		},
		{
			name:     "explicit credentials win over gitlab pat",
			rawURL:   "https://user:pass@gitlab.example.com/group/repo.git",
			opts:     Options{GitLabPAT: "glpat-secret", GitLabDomain: "gitlab.example.com"},
			wantURL:  "https://gitlab.example.com/group/repo.git",
			wantAuth: "user:pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrepareURL(tt.rawURL, tt.opts)
			if got.URL != tt.wantURL {
				t.Fatalf("URL=%q, want %q", got.URL, tt.wantURL)
			}
			if tt.wantAuth == "" {
				if len(got.Env) != 0 {
					t.Fatalf("Env=%v, want empty", got.Env)
				}
				return
			}
			assertAuthEnv(t, got.Env, "gitlab.example.com", tt.wantAuth)
			if strings.Contains(got.URL, tt.wantAuth) {
				t.Fatalf("clean URL contains auth payload: %q", got.URL)
			}
		})
	}
}

func TestNormalizeGitLabDomainHost(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "host", raw: "gitlab.example.com", want: "gitlab.example.com"},
		{name: "url", raw: "https://gitlab.example.com/group", want: "gitlab.example.com"},
		{name: "port", raw: "gitlab.example.com:8443", want: "gitlab.example.com"},
		{name: "path", raw: "gitlab.example.com/group", want: "gitlab.example.com"},
		{name: "empty", raw: " ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeGitLabDomainHost(tt.raw); got != tt.want {
				t.Fatalf("NormalizeGitLabDomainHost(%q)=%q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func assertAuthEnv(t *testing.T, env []string, host, userPass string) {
	t.Helper()
	if len(env) != 3 {
		t.Fatalf("Env len=%d, want 3: %v", len(env), env)
	}
	if env[0] != "GIT_CONFIG_COUNT=1" {
		t.Fatalf("Env[0]=%q", env[0])
	}
	if env[1] != "GIT_CONFIG_KEY_0=http.https://"+host+"/.extraHeader" {
		t.Fatalf("Env[1]=%q", env[1])
	}
	wantHeader := "GIT_CONFIG_VALUE_0=Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(userPass))
	if env[2] != wantHeader {
		t.Fatalf("Env[2]=%q, want %q", env[2], wantHeader)
	}
}
