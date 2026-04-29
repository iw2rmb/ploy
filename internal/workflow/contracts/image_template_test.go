package contracts

import (
	"strings"
	"testing"
)

func TestExpandImageTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		image   string
		stack   *StackExpectation
		env     map[string]string
		want    string
		wantErr string
	}{
		{
			name:  "plain image",
			image: "ghcr.io/acme/mig:latest",
			want:  "ghcr.io/acme/mig:latest",
		},
		{
			name:  "stack placeholders",
			image: "ghcr.io/acme/${stack.language}-${stack.release}-${stack.tool}:latest",
			stack: &StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			want:  "ghcr.io/acme/java-17-maven:latest",
		},
		{
			name:  "mixed stack and env placeholders",
			image: "${REG}/mig-${stack.tool}:${TAG}",
			stack: &StackExpectation{Language: "java", Release: "17", Tool: "gradle"},
			env: map[string]string{
				"REG": "registry.example/ploy",
				"TAG": "v1.2.3",
			},
			want: "registry.example/ploy/mig-gradle:v1.2.3",
		},
		{
			name:    "unknown stack placeholder",
			image:   "ghcr.io/acme/${stack.version}:latest",
			stack:   &StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			wantErr: "unknown stack placeholders: stack.version",
		},
		{
			name:    "missing stack value",
			image:   "ghcr.io/acme/${stack.language}-${stack.release}-${stack.tool}:latest",
			stack:   &StackExpectation{Language: "java", Release: "17"},
			wantErr: "unresolved stack placeholders: stack.tool",
		},
		{
			name:    "missing env value",
			image:   "${REG}/mig:latest",
			wantErr: "unresolved environment variables: REG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			envLookup := func(name string) (string, bool) {
				if tt.env == nil {
					return "", false
				}
				value, ok := tt.env[name]
				return value, ok
			}

			got, err := expandImageTemplateWithLookup(tt.image, tt.stack, envLookup)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpandImageTemplate() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ExpandImageTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}
