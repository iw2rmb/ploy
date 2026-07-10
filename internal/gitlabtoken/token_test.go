package gitlabtoken

import "testing"

func TestValidateRequest(t *testing.T) {
	token := "glpat-test-secret"
	hash := Hash(token)

	tests := []struct {
		name     string
		token    string
		wantHash string
		wantErr  bool
	}{
		{name: "token computes hash", token: token, wantHash: hash},
		{name: "empty token rejected", wantErr: true},
		{name: "blank token rejected", token: " \t\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHash, gotToken, err := ValidateRequest(tt.token)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateRequest() error = %v", err)
			}
			if tt.wantErr {
				return
			}
			if gotHash != tt.wantHash {
				t.Fatalf("hash = %q, want %q", gotHash, tt.wantHash)
			}
			if gotToken != token {
				t.Fatalf("token = %q, want %q", gotToken, token)
			}
		})
	}
}

func TestRunStatsMarkerRoundTrip(t *testing.T) {
	hash := Hash("glpat-test-secret")
	stats, err := RunStatsWithMarker(hash)
	if err != nil {
		t.Fatalf("RunStatsWithMarker() error = %v", err)
	}
	if got := HashFromRunStats(stats); got != hash {
		t.Fatalf("HashFromRunStats() = %q, want %q", got, hash)
	}
}
