package handlers

import "testing"

func TestClassifyGitLSRemoteFailure(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "auth denied",
			out:  "fatal: Authentication failed for 'https://gitlab.example.com/group/repo.git/'",
			want: "authentication failed or token rejected",
		},
		{
			name: "gitlab sign in redirect",
			out:  "fatal: unable to update url base from redirection:\n  redirect: https://gitlab.example.com/users/sign_in",
			want: "authentication failed or token rejected",
		},
		{
			name: "missing ref",
			out:  "fatal: couldn't find remote ref refs/heads/missing",
			want: "ref not found on remote",
		},
		{
			name: "missing project",
			out:  "remote: The project you were looking for could not be found.\nfatal: repository not found",
			want: "repository not found or access denied",
		},
		{
			name: "fallback",
			out:  "fatal: unexpected remote failure",
			want: "remote query failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyGitLSRemoteFailure([]byte(tt.out))
			if got != tt.want {
				t.Fatalf("classifyGitLSRemoteFailure() = %q, want %q", got, tt.want)
			}
		})
	}
}
