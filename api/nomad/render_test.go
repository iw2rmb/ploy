package nomad

import (
	"strings"
	"testing"
)

// TestTemplateForLaneAndLanguage tests language-specific template selection
func TestTemplateForLaneAndLanguage(t *testing.T) {
	tests := []struct {
		name             string
		lane             string
		language         string
		expectedTemplate string
	}{
		// Java/JVM languages
		{
			name:             "Lane C with Java",
			lane:             "C",
			language:         "java",
			expectedTemplate: "platform/nomad/lane-c-java.hcl",
		},
		{
			name:             "Lane C with JVM",
			lane:             "C",
			language:         "jvm",
			expectedTemplate: "platform/nomad/lane-c-java.hcl",
		},
		{
			name:             "Lane C with Kotlin",
			lane:             "C",
			language:         "kotlin",
			expectedTemplate: "platform/nomad/lane-c-java.hcl",
		},
		{
			name:             "Lane C with Scala",
			lane:             "C",
			language:         "scala",
			expectedTemplate: "platform/nomad/lane-c-java.hcl",
		},

		// Node.js languages
		{
			name:             "Lane C with Node",
			lane:             "C",
			language:         "node",
			expectedTemplate: "platform/nomad/lane-c-node.hcl",
		},
		{
			name:             "Lane C with NodeJS",
			lane:             "C",
			language:         "nodejs",
			expectedTemplate: "platform/nomad/lane-c-node.hcl",
		},
		{
			name:             "Lane C with JavaScript",
			lane:             "C",
			language:         "javascript",
			expectedTemplate: "platform/nomad/lane-c-node.hcl",
		},
		{
			name:             "Lane C with TypeScript",
			lane:             "C",
			language:         "typescript",
			expectedTemplate: "platform/nomad/lane-c-node.hcl",
		},

		// Future language templates (should fall back)
		{
			name:             "Lane C with Python (future)",
			lane:             "C",
			language:         "python",
			expectedTemplate: "platform/nomad/lane-c-osv.hcl",
		},
		{
			name:             "Lane C with Go (future)",
			lane:             "C",
			language:         "go",
			expectedTemplate: "platform/nomad/lane-c-osv.hcl",
		},

		// Other lanes (should use lane-specific templates)
		{
			name:             "Lane A with Java",
			lane:             "A",
			language:         "java",
			expectedTemplate: "platform/nomad/lane-a-unikraft.hcl",
		},
		{
			name:             "Lane D with Node",
			lane:             "D",
			language:         "node",
			expectedTemplate: "platform/nomad/lane-d-jail.hcl",
		},

		// Empty language (should fall back to generic)
		{
			name:             "Lane C with empty language",
			lane:             "C",
			language:         "",
			expectedTemplate: "platform/nomad/lane-c-osv.hcl",
		},

		// Case insensitive
		{
			name:             "Lane c with JAVA (case insensitive)",
			lane:             "c",
			language:         "JAVA",
			expectedTemplate: "platform/nomad/lane-c-java.hcl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := templateForLaneAndLanguage(tt.lane, tt.language)
			if result != tt.expectedTemplate {
				t.Errorf("templateForLaneAndLanguage(%s, %s) = %s, want %s",
					tt.lane, tt.language, result, tt.expectedTemplate)
			}
		})
	}
}

// TestRenderDataSetDefaults tests language-specific default settings
func TestRenderDataSetDefaults(t *testing.T) {
	tests := []struct {
		name                string
		language            string
		expectedJavaVersion string
		expectedNodeVersion string
		expectedMemoryLimit int
	}{
		{
			name:                "Java defaults",
			language:            "java",
			expectedJavaVersion: "17",
			expectedNodeVersion: "",
			expectedMemoryLimit: 256,
		},
		{
			name:                "Node.js defaults",
			language:            "node",
			expectedJavaVersion: "",
			expectedNodeVersion: "18",
			expectedMemoryLimit: 512, // Node.js gets more memory
		},
		{
			name:                "TypeScript defaults",
			language:            "typescript",
			expectedJavaVersion: "",
			expectedNodeVersion: "18",
			expectedMemoryLimit: 512,
		},
		{
			name:                "Unknown language defaults",
			language:            "unknown",
			expectedJavaVersion: "",
			expectedNodeVersion: "",
			expectedMemoryLimit: 256,
		},
		{
			name:                "Empty language defaults",
			language:            "",
			expectedJavaVersion: "",
			expectedNodeVersion: "",
			expectedMemoryLimit: 256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := RenderData{
				Language: tt.language,
			}

			data.SetDefaults()

			if data.JavaVersion != tt.expectedJavaVersion {
				t.Errorf("Expected JavaVersion %s, got %s", tt.expectedJavaVersion, data.JavaVersion)
			}

			if data.NodeVersion != tt.expectedNodeVersion {
				t.Errorf("Expected NodeVersion %s, got %s", tt.expectedNodeVersion, data.NodeVersion)
			}

			if data.MemoryLimit != tt.expectedMemoryLimit {
				t.Errorf("Expected MemoryLimit %d, got %d", tt.expectedMemoryLimit, data.MemoryLimit)
			}

			// Verify enterprise features default to disabled for non-platform apps
			if data.VaultEnabled {
				t.Error("Expected VaultEnabled to be false by default")
			}
			if data.ConnectEnabled {
				t.Error("Expected ConnectEnabled to be false by default")
			}
			if data.VolumeEnabled {
				t.Error("Expected VolumeEnabled to be false by default")
			}
			if data.ConsulConfigEnabled {
				t.Error("Expected ConsulConfigEnabled to be false by default")
			}

			// Verify debug is disabled by default
			if data.DebugEnabled {
				t.Error("Expected DebugEnabled to be false by default")
			}
		})
	}
}

// TestRenderTemplateFilenames tests language-specific template selection
func TestRenderTemplateFilenames(t *testing.T) {
	tests := []struct {
		name             string
		appName          string
		lane             string
		language         string
		expectedTemplate string
	}{
		{
			name:             "Java app template selection",
			appName:          "my-app",
			lane:             "C",
			language:         "java",
			expectedTemplate: "platform/nomad/lane-c-java.hcl",
		},
		{
			name:             "Node.js app template selection",
			appName:          "node-service",
			lane:             "C",
			language:         "node",
			expectedTemplate: "platform/nomad/lane-c-node.hcl",
		},
		{
			name:             "No language template selection",
			appName:          "generic-app",
			lane:             "C",
			language:         "",
			expectedTemplate: "platform/nomad/lane-c-osv.hcl",
		},
		{
			name:             "Other lane template selection",
			appName:          "unikernel-app",
			lane:             "A",
			language:         "java",
			expectedTemplate: "platform/nomad/lane-a-unikraft.hcl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := RenderData{
				App:      tt.appName,
				Language: tt.language,
			}

			// Call RenderTemplate to generate an output file using embedded templates
			out, err := RenderTemplate(tt.lane, data)
			if err != nil {
				t.Fatalf("RenderTemplate failed: %v", err)
			}
			if out == "" || !strings.HasSuffix(out, ".hcl") {
				t.Errorf("expected output file path ending with .hcl, got %q", out)
			}
		})
	}
}

// TestTemplateForLane tests legacy template selection still works
func TestTemplateForLane(t *testing.T) {
	tests := []struct {
		lane     string
		expected string
	}{
		{"A", "platform/nomad/lane-a-unikraft.hcl"},
		{"B", "platform/nomad/lane-b-unikraft-posix.hcl"},
		{"C", "platform/nomad/lane-c-osv.hcl"},
		{"D", "platform/nomad/lane-d-jail.hcl"},
		{"E", "platform/nomad/lane-e-oci-kontain.hcl"},
		{"F", "platform/nomad/lane-f-vm.hcl"},
		{"invalid", "platform/nomad/lane-c-osv.hcl"},
		{"", "platform/nomad/lane-c-osv.hcl"},
	}

	for _, tt := range tests {
		t.Run("Lane "+tt.lane, func(t *testing.T) {
			result := templateForLane(tt.lane)
			if result != tt.expected {
				t.Errorf("templateForLane(%s) = %s, want %s", tt.lane, result, tt.expected)
			}
		})
	}
}
