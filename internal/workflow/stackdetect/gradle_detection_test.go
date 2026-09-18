package stackdetect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectGradle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fileName     string
		content      string
		extraFiles   map[string]string
		wantRelease  string
		wantEvidence []EvidenceItem
		wantError    bool
		wantErrorMsg string
	}{
		{
			name:     "ext properties with explicit compatibility",
			fileName: "build.gradle",
			content: `import static org.gradle.api.JavaVersion.VERSION_11

plugins { id 'java' }

sourceCompatibility = VERSION_11
targetCompatibility = VERSION_11

ext['log4j2.version'] = '2.16.0'
`,
			wantRelease: "11",
		},
		{
			name:     "source and target compatibility differ",
			fileName: "build.gradle",
			content: `sourceCompatibility = 17
targetCompatibility = 21`,
			wantError:    true,
			wantErrorMsg: "sourceCompatibility and targetCompatibility differ",
			wantEvidence: []EvidenceItem{
				{Path: "build.gradle", Key: "sourceCompatibility", Value: "17"},
				{Path: "build.gradle", Key: "targetCompatibility", Value: "21"},
			},
		},
		{
			name:     "Kotlin JVM target forms differ",
			fileName: "build.gradle.kts",
			content: `kotlinOptions.jvmTarget = "17"
kotlinOptions { jvmTarget = "21" }`,
			wantError:    true,
			wantErrorMsg: "kotlinOptions.jvmTarget differs between assignments",
			wantEvidence: []EvidenceItem{
				{Path: "build.gradle.kts", Key: "kotlinOptions.jvmTarget", Value: "17"},
				{Path: "build.gradle.kts", Key: "kotlinOptions.jvmTarget", Value: "21"},
			},
		},
		{
			name:     "toolchain languageVersion assign",
			fileName: "build.gradle",
			content: `
plugins { id "java" }

java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(17)
    }
}
`,
			wantRelease: "17",
		},
		{
			name:     "toolchain languageVersion assign JavaVersion constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion = JavaVersion.VERSION_21
    }
}
`,
			wantRelease: "21",
		},
		{
			name:     "toolchain languageVersion assign unqualified constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion = VERSION_17
    }
}
`,
			wantRelease: "17",
		},
		{
			name:     "toolchain languageVersion assign numeric",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion = 11
    }
}
`,
			wantRelease: "11",
		},
		{
			name:     "toolchain languageVersion set KTS",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(JavaLanguageVersion.of("21"))
    }
}
`,
			wantRelease: "21",
		},
		{
			name:     "toolchain languageVersion set JavaVersion constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(JavaVersion.VERSION_17)
    }
}
`,
			wantRelease: "17",
		},
		{
			name:     "toolchain languageVersion set unqualified constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(VERSION_11)
    }
}
`,
			wantRelease: "11",
		},
		{
			name:     "toolchain languageVersion set numeric",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(21)
    }
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion assignment qualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion = JavaVersion.VERSION_21
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion assignment JavaLanguageVersion factory",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion = JavaLanguageVersion.of(21)
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion assignment unqualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion = VERSION_17
}
`,
			wantRelease: "17",
		},
		{
			name:     "dependency manager javaVersion set qualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(JavaVersion.VERSION_21)
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion set unqualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(VERSION_17)
}
`,
			wantRelease: "17",
		},
		{
			name:     "dependency manager javaVersion set numeric",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(11)
}
`,
			wantRelease: "11",
		},
		{
			name:     "dependency manager javaVersion set JavaLanguageVersion factory",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(JavaLanguageVersion.of("21"))
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion forms differ",
			fileName: "build.gradle.kts",
			content: `dependencyManagerRootExtension {
    javaVersion = 17
    javaVersion.set(21)
}`,
			wantError:    true,
			wantErrorMsg: "javaVersion differs between assignments",
			wantEvidence: []EvidenceItem{
				{Path: "build.gradle.kts", Key: "javaVersion", Value: "17"},
				{Path: "build.gradle.kts", Key: "javaVersion", Value: "21"},
			},
		},
		{
			name:     "toolchain factory setter from Gradle property",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

val projectJavaVersion: String by project

java {
    toolchain {
        languageVersion.set(JavaLanguageVersion.of(projectJavaVersion))
    }
}
`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantRelease: "25",
			wantEvidence: []EvidenceItem{
				{Path: "build.gradle.kts", Key: "java.toolchain.languageVersion", Value: "25"},
				{Path: "gradle.properties", Key: "projectJavaVersion", Value: "25"},
			},
		},
		{
			name:     "toolchain factory assignment from normalized Gradle property",
			fileName: "build.gradle.kts",
			content: `
java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(jdkVersion)
    }
}
`,
			extraFiles: map[string]string{
				"gradle.properties": " # old value\njdkVersion=17\n ! current value follows\njdkVersion : \"1.8\"\n",
			},
			wantRelease: "8",
			wantEvidence: []EvidenceItem{
				{Path: "build.gradle.kts", Key: "java.toolchain.languageVersion", Value: "8"},
				{Path: "gradle.properties", Key: "jdkVersion", Value: "8"},
			},
		},
		{
			name:     "toolchain direct setter from Gradle property",
			fileName: "build.gradle.kts",
			content:  `java { toolchain { languageVersion.set(jdkVersion) } }`,
			extraFiles: map[string]string{
				"gradle.properties": "jdkVersion=21\n",
			},
			wantRelease: "21",
		},
		{
			name:     "dependency manager assignment from Gradle property",
			fileName: "build.gradle.kts",
			content: `dependencyManagerRootExtension {
    javaVersion = projectJavaVersion
}`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantRelease: "25",
		},
		{
			name:     "dependency manager setter from Gradle property",
			fileName: "build.gradle.kts",
			content:  `dependencyManagerRootExtension { javaVersion.set(projectJavaVersion) }`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantRelease: "25",
		},
		{
			name:     "dependency manager factory assignment from Gradle property",
			fileName: "build.gradle.kts",
			content:  `dependencyManagerRootExtension { javaVersion = JavaLanguageVersion.of(projectJavaVersion) }`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantRelease: "25",
		},
		{
			name:     "dependency manager factory setter from Gradle property",
			fileName: "build.gradle.kts",
			content:  `dependencyManagerRootExtension { javaVersion.set(JavaLanguageVersion.of(projectJavaVersion)) }`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantRelease: "25",
		},
		{
			name:     "explicit compatibility precedes Gradle property toolchain",
			fileName: "build.gradle.kts",
			content: `sourceCompatibility = 17
java { toolchain { languageVersion.set(JavaLanguageVersion.of(projectJavaVersion)) } }`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantRelease: "17",
			wantEvidence: []EvidenceItem{
				{Path: "build.gradle.kts", Key: "sourceCompatibility", Value: "17"},
			},
		},
		{
			name:     "literal and Gradle property toolchain values differ",
			fileName: "build.gradle.kts",
			content: `java { toolchain {
    languageVersion = JavaLanguageVersion.of(17)
    languageVersion.set(JavaLanguageVersion.of(projectJavaVersion))
} }`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantError:    true,
			wantErrorMsg: "toolchain languageVersion differs between assignments",
			wantEvidence: []EvidenceItem{
				{Path: "build.gradle.kts", Key: "java.toolchain.languageVersion", Value: "17"},
				{Path: "build.gradle.kts", Key: "java.toolchain.languageVersion", Value: "25"},
				{Path: "gradle.properties", Key: "projectJavaVersion", Value: "25"},
			},
		},
		{
			name:     "missing Gradle property",
			fileName: "build.gradle.kts",
			content:  `java { toolchain { languageVersion.set(JavaLanguageVersion.of(projectJavaVersion)) } }`,
			extraFiles: map[string]string{
				"gradle.properties": "otherVersion=25\n",
			},
			wantError: true,
		},
		{
			name:     "non-version Gradle property",
			fileName: "build.gradle.kts",
			content:  `java { toolchain { languageVersion.set(JavaLanguageVersion.of(projectJavaVersion)) } }`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=17\nprojectJavaVersion=latest\n",
			},
			wantError: true,
		},
		{
			name:     "Gradle property provider remains unsupported",
			fileName: "build.gradle.kts",
			content:  `java { toolchain { languageVersion.set(providers.gradleProperty("projectJavaVersion")) } }`,
			extraFiles: map[string]string{
				"gradle.properties": "projectJavaVersion=25\n",
			},
			wantError: true,
		},
		{
			name:     "version catalog jvm target",
			fileName: "build.gradle",
			content: `
plugins { id "java" }
`,
			extraFiles: map[string]string{
				"gradle/libs.versions.toml": `
[versions]
jvmTarget = "17"
`,
			},
			wantRelease: "17",
			wantEvidence: []EvidenceItem{
				{Path: "gradle/libs.versions.toml", Key: "versions.jvmTarget", Value: "17"},
			},
		},
		{
			name:     "version catalog JDK",
			fileName: "build.gradle.kts",
			content: `
plugins { id("convention.kotlin") }
`,
			extraFiles: map[string]string{
				"gradle/libs.versions.toml": `
[versions]
jdk = "21"
`,
			},
			wantRelease: "21",
			wantEvidence: []EvidenceItem{
				{Path: "gradle/libs.versions.toml", Key: "versions.jdk", Value: "21"},
			},
		},
		{
			name:     "matching version catalog Java keys",
			fileName: "build.gradle.kts",
			content:  `plugins { java }`,
			extraFiles: map[string]string{
				"gradle/libs.versions.toml": `
[versions]
jvmTarget = "21"
jdk = "21"
`,
			},
			wantRelease: "21",
			wantEvidence: []EvidenceItem{
				{Path: "gradle/libs.versions.toml", Key: "versions.jvmTarget", Value: "21"},
				{Path: "gradle/libs.versions.toml", Key: "versions.jdk", Value: "21"},
			},
		},
		{
			name:     "conflicting version catalog Java keys",
			fileName: "build.gradle.kts",
			content:  `plugins { java }`,
			extraFiles: map[string]string{
				"gradle/libs.versions.toml": `
[versions]
jvmTarget = "17"
jdk = "21"
`,
			},
			wantError:    true,
			wantErrorMsg: "version catalog jvmTarget and jdk differ",
			wantEvidence: []EvidenceItem{
				{Path: "gradle/libs.versions.toml", Key: "versions.jvmTarget", Value: "17"},
				{Path: "gradle/libs.versions.toml", Key: "versions.jdk", Value: "21"},
			},
		},
		{
			name:     "malformed version catalog",
			fileName: "build.gradle",
			content: `
plugins { id "java" }
`,
			extraFiles: map[string]string{
				"gradle/libs.versions.toml": `
[versions
jvmTarget = "17"
`,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			gradlePath := filepath.Join(workspace, tt.fileName)
			if err := os.WriteFile(gradlePath, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write %s: %v", tt.fileName, err)
			}
			for rel, content := range tt.extraFiles {
				path := filepath.Join(workspace, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
			}

			obs, err := detectGradle(context.Background(), workspace, gradlePath)
			if tt.wantError {
				if err == nil {
					t.Fatal("detectGradle error = nil, want non-nil")
				}
				if tt.wantErrorMsg != "" {
					var detectionErr *DetectionError
					if !errors.As(err, &detectionErr) {
						t.Fatalf("error = %T, want *DetectionError", err)
					}
					if detectionErr.Reason != "unknown" || detectionErr.Message != tt.wantErrorMsg {
						t.Fatalf("detection error = %#v, want reason unknown and message %q", detectionErr, tt.wantErrorMsg)
					}
					if !reflect.DeepEqual(detectionErr.Evidence, tt.wantEvidence) {
						t.Fatalf("error evidence = %#v, want %#v", detectionErr.Evidence, tt.wantEvidence)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("detectGradle error: %v", err)
			}
			if obs == nil || obs.Release == nil {
				t.Fatal("nil observation or release")
			}
			if got := *obs.Release; got != tt.wantRelease {
				t.Errorf("release = %q, want %q", got, tt.wantRelease)
			}
			if obs.Tool != "gradle" {
				t.Errorf("tool = %q, want %q", obs.Tool, "gradle")
			}
			if obs.Language != "java" {
				t.Errorf("language = %q, want %q", obs.Language, "java")
			}
			if tt.wantEvidence != nil && !reflect.DeepEqual(obs.Evidence, tt.wantEvidence) {
				t.Errorf("evidence = %#v, want %#v", obs.Evidence, tt.wantEvidence)
			}
		})
	}
}
