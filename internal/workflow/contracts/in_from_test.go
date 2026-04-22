package contracts

import "testing"

func TestParseInFromURI(t *testing.T) {
	tests := []struct {
		name           string
		raw            string
		wantSourceName string
		wantSourceType string
		wantOutPath    string
		wantErr        bool
	}{
		{
			name:           "type selector",
			raw:            "sbom://out/java.classpath",
			wantSourceName: "",
			wantSourceType: "sbom",
			wantOutPath:    "/out/java.classpath",
		},
		{
			name:           "named type selector",
			raw:            "java-analysis@mig://out/dependency-usage.nofilter.json",
			wantSourceName: "java-analysis",
			wantSourceType: "mig",
			wantOutPath:    "/out/dependency-usage.nofilter.json",
		},
		{
			name:    "invalid named unknown type",
			raw:     "java-analysis@unknown://out/dependency-usage.nofilter.json",
			wantErr: true,
		},
		{
			name:    "invalid multiple at",
			raw:     "name@mig@x://out/value.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInFromURI(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseInFromURI() error = %v", err)
			}
			if got.SourceName != tt.wantSourceName {
				t.Fatalf("SourceName = %q, want %q", got.SourceName, tt.wantSourceName)
			}
			if got.SourceType.String() != tt.wantSourceType {
				t.Fatalf("SourceType = %q, want %q", got.SourceType, tt.wantSourceType)
			}
			if got.OutPath != tt.wantOutPath {
				t.Fatalf("OutPath = %q, want %q", got.OutPath, tt.wantOutPath)
			}
		})
	}
}

func TestNormalizeInFromTarget_Default(t *testing.T) {
	got, err := NormalizeInFromTarget("", "/out/dependency-usage.nofilter.json")
	if err != nil {
		t.Fatalf("NormalizeInFromTarget() error = %v", err)
	}
	if got != "/in/dependency-usage.nofilter.json" {
		t.Fatalf("target = %q, want %q", got, "/in/dependency-usage.nofilter.json")
	}
}

func TestNormalizeInFromTarget_ExplicitInPath(t *testing.T) {
	got, err := NormalizeInFromTarget("/in/custom/path.json", "/out/source.json")
	if err != nil {
		t.Fatalf("NormalizeInFromTarget() error = %v", err)
	}
	if got != "/in/custom/path.json" {
		t.Fatalf("target = %q, want %q", got, "/in/custom/path.json")
	}
}

func TestParseMigSpecJSON_InFromValidation(t *testing.T) {
	t.Run("valid type selector without step name", func(t *testing.T) {
		input := `{
			"steps": [
				{"name": "extract-usage", "image": "img1"},
				{"image": "img2", "in_from": [
					{"from": "sbom://out/java.classpath"}
				]}
			]
		}`
		spec, err := ParseMigSpecJSON([]byte(input))
		if err != nil {
			t.Fatalf("ParseMigSpecJSON() error = %v", err)
		}
		if got := spec.Steps[1].InFrom[0].To; got != "/in/java.classpath" {
			t.Fatalf("steps[1].in_from[0].to = %q, want %q", got, "/in/java.classpath")
		}
	})

	t.Run("future named mig selector rejected", func(t *testing.T) {
		input := `{
			"steps": [
				{"name": "extract-usage", "image": "img1", "in_from": [
					{"from": "compose-deprecations@mig://out/dependency-usage.nofilter.json"}
				]},
				{"name": "compose-deprecations", "image": "img2"}
			]
		}`
		_, err := ParseMigSpecJSON([]byte(input))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("named non-mig selector rejected", func(t *testing.T) {
		input := `{
			"steps": [
				{"name": "extract-usage", "image": "img1"},
				{"image": "img2", "in_from": [
					{"from": "pre@sbom://out/java.classpath"}
				]}
			]
		}`
		_, err := ParseMigSpecJSON([]byte(input))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("duplicate step name rejected", func(t *testing.T) {
		input := `{
			"steps": [
				{"name": "extract-usage", "image": "img1"},
				{"name": "extract-usage", "image": "img2", "in_from": [
					{"from": "extract-usage@mig://out/dependency-usage.nofilter.json"}
				]}
			]
		}`
		_, err := ParseMigSpecJSON([]byte(input))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("duplicate step name allowed when no named mig selector", func(t *testing.T) {
		input := `{
			"steps": [
				{"name": "dup", "image": "img1"},
				{"name": "dup", "image": "img2", "in_from": [
					{"from": "sbom://out/java.classpath"}
				]}
			]
		}`
		if _, err := ParseMigSpecJSON([]byte(input)); err != nil {
			t.Fatalf("ParseMigSpecJSON() error = %v", err)
		}
	})
}
