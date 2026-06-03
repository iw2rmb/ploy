package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJavaGradleTrimmerHandler(t *testing.T) {
	t.Parallel()

	gradleLog := strings.Join([]string{
		"/workspace/src/main/java/a/A.java:10: error: cannot find symbol",
		"  symbol:   class Missing",
		"1 errors",
		"* What went wrong:",
		"Execution failed for task ':compileJava'.",
		"> Compilation failed; see the compiler error output for details.",
		"BUILD FAILED in 1s",
		"",
	}, "\n")

	tests := []struct {
		name        string
		body        string
		contentType string
		accept      string
		target      string
		wantStatus  int
		wantType    string
		wantCD      string
		wantBody    []string
	}{
		{
			name:        "json request returns json",
			body:        `{"log":` + quoteJSON(gradleLog) + `}`,
			contentType: "application/json",
			target:      "/v1/trimmer/java/gradle?format=json",
			wantStatus:  http.StatusOK,
			wantType:    "application/json",
			wantCD:      "attachment; filename=gradle-trimmed.json",
			wantBody:    []string{`"tool":"gradle"`, `"evidence"`, `"task":"compileJava"`},
		},
		{
			name:        "plain text request returns yaml by format",
			body:        gradleLog,
			contentType: "text/plain",
			target:      "/v1/trimmer/java/gradle?format=yaml",
			wantStatus:  http.StatusOK,
			wantType:    "application/x-yaml",
			wantCD:      "attachment; filename=gradle-trimmed.yaml",
			wantBody:    []string{"tool: gradle", "evidence:", "task: compileJava"},
		},
		{
			name:        "plain text request returns yaml by accept preference",
			body:        gradleLog,
			contentType: "text/plain; charset=utf-8",
			accept:      "application/json;q=0.2, application/x-yaml;q=0.9",
			target:      "/v1/trimmer/java/gradle",
			wantStatus:  http.StatusOK,
			wantType:    "application/x-yaml",
			wantCD:      "attachment; filename=gradle-trimmed.yaml",
			wantBody:    []string{"tool: gradle"},
		},
		{
			name:        "invalid json",
			body:        `{"log":`,
			contentType: "application/json",
			target:      "/v1/trimmer/java/gradle",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unknown json field",
			body:        `{"log":"x","extra":true}`,
			contentType: "application/json",
			target:      "/v1/trimmer/java/gradle",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "empty log",
			body:        `{"log":"   "}`,
			contentType: "application/json",
			target:      "/v1/trimmer/java/gradle",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unsupported format",
			body:        gradleLog,
			contentType: "text/plain",
			target:      "/v1/trimmer/java/gradle?format=xml",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unsupported content type",
			body:        gradleLog,
			contentType: "application/octet-stream",
			target:      "/v1/trimmer/java/gradle",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "oversized plain text",
			body:        strings.Repeat("x", ingestMaxDataSize+1),
			contentType: "text/plain",
			target:      "/v1/trimmer/java/gradle",
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
		{
			name:        "oversized decoded json log",
			body:        `{"log":` + quoteJSON(strings.Repeat("x", ingestMaxDataSize+1)) + `}`,
			contentType: "application/json",
			target:      "/v1/trimmer/java/gradle",
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, tt.target, strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			rr := httptest.NewRecorder()

			javaGradleTrimmerHandler().ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rr.Code, tt.wantStatus, rr.Body.String())
			}
			if tt.wantType != "" && !strings.HasPrefix(rr.Header().Get("Content-Type"), tt.wantType) {
				t.Fatalf("Content-Type = %q, want prefix %q", rr.Header().Get("Content-Type"), tt.wantType)
			}
			if tt.wantCD != "" && rr.Header().Get("Content-Disposition") != tt.wantCD {
				t.Fatalf("Content-Disposition = %q, want %q", rr.Header().Get("Content-Disposition"), tt.wantCD)
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(rr.Body.String(), want) {
					t.Fatalf("body missing %q:\n%s", want, rr.Body.String())
				}
			}
		})
	}
}

// Decoded top-level key assertions are clearer outside the request matrix.
func TestJavaGradleTrimmerHandlerOmitsCompleteCompilerMessage(t *testing.T) {
	t.Parallel()

	gradleLog := strings.Join([]string{
		"/workspace/src/main/java/a/A.java:10: error: cannot find symbol",
		"  symbol:   class Missing",
		"1 errors",
		"* What went wrong:",
		"Execution failed for task ':compileJava'.",
		"> Compilation failed; see the compiler error output for details.",
		"BUILD FAILED in 1s",
		"",
	}, "\n")
	req := httptest.NewRequest(http.MethodPost, "/v1/trimmer/java/gradle?format=json", strings.NewReader(gradleLog))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()

	javaGradleTrimmerHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v\n%s", err, rr.Body.String())
	}
	if _, ok := body["message"]; ok {
		t.Fatalf("unexpected root message in complete compiler result:\n%s", rr.Body.String())
	}
	if _, ok := body["evidence"]; !ok {
		t.Fatalf("expected evidence in response:\n%s", rr.Body.String())
	}
}

func quoteJSON(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(data)
}
