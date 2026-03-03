package contracts

import (
	"strings"
	"testing"
)

func TestParseORWCLIInputFromEnv(t *testing.T) {
	t.Parallel()

	validRecipe := map[string]string{
		ORWRecipeGroupEnv:     "org.openrewrite.recipe",
		ORWRecipeArtifactEnv:  "rewrite-migrate-java",
		ORWRecipeVersionEnv:   "3.20.0",
		ORWRecipeClassnameEnv: "org.openrewrite.java.migrate.UpgradeToJava17",
	}

	t.Run("rejects missing required recipe env", func(t *testing.T) {
		t.Parallel()

		_, err := ParseORWCLIInputFromEnv(map[string]string{})
		if err == nil {
			t.Fatal("expected error for missing recipe env")
		}
		if !strings.Contains(err.Error(), ORWRecipeGroupEnv) {
			t.Fatalf("error = %q, want mention of %s", err.Error(), ORWRecipeGroupEnv)
		}
	})

	t.Run("parses full env contract", func(t *testing.T) {
		t.Parallel()

		env := map[string]string{
			ORWRecipeGroupEnv:     validRecipe[ORWRecipeGroupEnv],
			ORWRecipeArtifactEnv:  validRecipe[ORWRecipeArtifactEnv],
			ORWRecipeVersionEnv:   validRecipe[ORWRecipeVersionEnv],
			ORWRecipeClassnameEnv: validRecipe[ORWRecipeClassnameEnv],

			ORWReposEnv:             "https://repo1.maven.org/maven2, https://repo.example.local/maven",
			ORWRepoUsernameEnv:      "robot",
			ORWRepoPasswordEnv:      "secret",
			ORWActiveRecipesEnv:     "com.example.RecipeA, com.example.RecipeB",
			ORWFailOnUnsupportedEnv: "false",
		}

		got, err := ParseORWCLIInputFromEnv(env)
		if err != nil {
			t.Fatalf("ParseORWCLIInputFromEnv returned error: %v", err)
		}

		if got.Recipe.Group != validRecipe[ORWRecipeGroupEnv] {
			t.Fatalf("Recipe.Group = %q, want %q", got.Recipe.Group, validRecipe[ORWRecipeGroupEnv])
		}
		if got.Recipe.Classname != validRecipe[ORWRecipeClassnameEnv] {
			t.Fatalf("Recipe.Classname = %q, want %q", got.Recipe.Classname, validRecipe[ORWRecipeClassnameEnv])
		}
		if len(got.Repositories) != 2 {
			t.Fatalf("Repositories len = %d, want 2", len(got.Repositories))
		}
		if got.Repositories[1] != "https://repo.example.local/maven" {
			t.Fatalf("Repositories[1] = %q, want repo.example.local", got.Repositories[1])
		}
		if len(got.ActiveRecipes) != 2 {
			t.Fatalf("ActiveRecipes len = %d, want 2", len(got.ActiveRecipes))
		}
		if got.FailOnUnsupported {
			t.Fatal("FailOnUnsupported = true, want false")
		}
	})

	t.Run("defaults fail on unsupported to true", func(t *testing.T) {
		t.Parallel()

		got, err := ParseORWCLIInputFromEnv(validRecipe)
		if err != nil {
			t.Fatalf("ParseORWCLIInputFromEnv returned error: %v", err)
		}
		if !got.FailOnUnsupported {
			t.Fatal("FailOnUnsupported = false, want true")
		}
	})

	t.Run("rejects invalid fail on unsupported value", func(t *testing.T) {
		t.Parallel()

		env := map[string]string{
			ORWRecipeGroupEnv:       validRecipe[ORWRecipeGroupEnv],
			ORWRecipeArtifactEnv:    validRecipe[ORWRecipeArtifactEnv],
			ORWRecipeVersionEnv:     validRecipe[ORWRecipeVersionEnv],
			ORWRecipeClassnameEnv:   validRecipe[ORWRecipeClassnameEnv],
			ORWFailOnUnsupportedEnv: "sometimes",
		}
		_, err := ParseORWCLIInputFromEnv(env)
		if err == nil {
			t.Fatal("expected error for invalid ORW_FAIL_ON_UNSUPPORTED")
		}
		if !strings.Contains(err.Error(), ORWFailOnUnsupportedEnv) {
			t.Fatalf("error = %q, want mention of %s", err.Error(), ORWFailOnUnsupportedEnv)
		}
	})

	t.Run("rejects partial repository credentials", func(t *testing.T) {
		t.Parallel()

		env := map[string]string{
			ORWRecipeGroupEnv:     validRecipe[ORWRecipeGroupEnv],
			ORWRecipeArtifactEnv:  validRecipe[ORWRecipeArtifactEnv],
			ORWRecipeVersionEnv:   validRecipe[ORWRecipeVersionEnv],
			ORWRecipeClassnameEnv: validRecipe[ORWRecipeClassnameEnv],
			ORWRepoUsernameEnv:    "robot",
		}
		_, err := ParseORWCLIInputFromEnv(env)
		if err == nil {
			t.Fatal("expected credentials pair error")
		}
		if !strings.Contains(err.Error(), ORWRepoPasswordEnv) {
			t.Fatalf("error = %q, want mention of %s", err.Error(), ORWRepoPasswordEnv)
		}
	})
}

func TestParseORWCLIReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		payload     string
		wantErrPart string
		wantSuccess bool
		wantKind    ORWCLIErrorKind
		wantReason  string
	}{
		{
			name:        "success report",
			payload:     `{"success":true}`,
			wantSuccess: true,
		},
		{
			name:        "unsupported failure report",
			payload:     `{"success":false,"error_kind":"unsupported","reason":"type-attribution-unavailable","message":"Type attribution is unavailable for this repository"}`,
			wantSuccess: false,
			wantKind:    ORWCLIErrorKindUnsupported,
			wantReason:  ORWCLIReasonTypeAttributionUnavailable,
		},
		{
			name:        "missing success rejected",
			payload:     `{"error_kind":"execution","message":"failed to run"}`,
			wantErrPart: "success is required",
		},
		{
			name:        "unknown field rejected",
			payload:     `{"success":true,"extra":"nope"}`,
			wantErrPart: "unknown field",
		},
		{
			name:        "unsupported requires deterministic reason",
			payload:     `{"success":false,"error_kind":"unsupported","message":"unsupported without reason"}`,
			wantErrPart: "reason is required",
		},
		{
			name:        "failure requires message",
			payload:     `{"success":false,"error_kind":"execution"}`,
			wantErrPart: "message is required",
		},
		{
			name:        "unknown error kind rejected",
			payload:     `{"success":false,"error_kind":"boom","message":"failed"}`,
			wantErrPart: "error_kind invalid",
		},
		{
			name:        "success forbids error kind",
			payload:     `{"success":true,"error_kind":"execution"}`,
			wantErrPart: "error_kind must be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseORWCLIReport([]byte(tt.payload))
			if tt.wantErrPart != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErrPart)
				}
				if !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("error = %q, want part %q", err.Error(), tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseORWCLIReport returned error: %v", err)
			}
			if got.Success != tt.wantSuccess {
				t.Fatalf("Success = %v, want %v", got.Success, tt.wantSuccess)
			}
			if got.ErrorKind != tt.wantKind {
				t.Fatalf("ErrorKind = %q, want %q", got.ErrorKind, tt.wantKind)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}
