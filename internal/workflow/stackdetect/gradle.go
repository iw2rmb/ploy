package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Regex patterns for Gradle version detection.
var (
	// sourceCompatibility = "17" or sourceCompatibility = 17
	// sourceCompatibility = JavaVersion.VERSION_17 or sourceCompatibility = VERSION_17
	sourceCompatibilityRegex = regexp.MustCompile(`sourceCompatibility\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	// targetCompatibility = "17" or targetCompatibility = 17
	// targetCompatibility = JavaVersion.VERSION_17 or targetCompatibility = VERSION_17
	targetCompatibilityRegex = regexp.MustCompile(`targetCompatibility\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	// kotlinOptions.jvmTarget = "17" or kotlinOptions.jvmTarget = JavaVersion.VERSION_17
	kotlinOptionsJvmTargetDirectRegex = regexp.MustCompile(`kotlinOptions\.jvmTarget\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	// kotlinOptions { ... jvmTarget = "17" } or kotlinOptions { ... jvmTarget = JavaVersion.VERSION_17 }
	kotlinOptionsJvmTargetBlockRegex = regexp.MustCompile(`(?s)kotlinOptions\s*\{.*?jvmTarget\s*=\s*(?:"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`)

	javaVersionValuePattern = `(?:JavaLanguageVersion\.of\(\s*"?(\d+(?:\.\d+)?)"?\s*\)|"?(\d+(?:\.\d+)?)"?|(?:JavaVersion\.)?VERSION_([0-9_]+))`
	gradlePropertyPattern   = `([A-Za-z_][A-Za-z0-9_]*)`

	// java { toolchain { languageVersion = JavaLanguageVersion.of(17) } }
	// java { toolchain { languageVersion = JavaVersion.VERSION_17 } }
	toolchainLanguageVersionAssignRegex         = regexp.MustCompile(`(?s)toolchain\s*\{.*?\blanguageVersion\s*=\s*` + javaVersionValuePattern)
	toolchainLanguageVersionAssignPropertyRegex = regexp.MustCompile(`(?s)toolchain\s*\{.*?\blanguageVersion\s*=\s*JavaLanguageVersion\.of\(\s*` + gradlePropertyPattern + `\s*\)`)

	// java { toolchain { languageVersion.set(JavaLanguageVersion.of(17)) } }
	// java { toolchain { languageVersion.set(JavaVersion.VERSION_17) } }
	toolchainLanguageVersionSetRegex                = regexp.MustCompile(`(?s)toolchain\s*\{.*?\blanguageVersion\.set\(\s*` + javaVersionValuePattern + `\s*\)`)
	toolchainLanguageVersionSetFactoryPropertyRegex = regexp.MustCompile(`(?s)toolchain\s*\{.*?\blanguageVersion\.set\(\s*JavaLanguageVersion\.of\(\s*` + gradlePropertyPattern + `\s*\)\s*\)`)
	toolchainLanguageVersionSetPropertyRegex        = regexp.MustCompile(`(?s)toolchain\s*\{.*?\blanguageVersion\.set\(\s*` + gradlePropertyPattern + `\s*\)`)

	// dependencyManagerRootExtension { javaVersion = JavaVersion.VERSION_21 }.
	// Also supports JavaLanguageVersion.of(21), unqualified VERSION_21, and numeric values.
	javaVersionAssignmentRegex                = regexp.MustCompile(`\bjavaVersion\s*=\s*` + javaVersionValuePattern)
	javaVersionAssignmentFactoryPropertyRegex = regexp.MustCompile(`\bjavaVersion\s*=\s*JavaLanguageVersion\.of\(\s*` + gradlePropertyPattern + `\s*\)`)
	javaVersionAssignmentPropertyRegex        = regexp.MustCompile(`(?m)\bjavaVersion\s*=\s*` + gradlePropertyPattern + `\s*(?:$|[;}])`)

	// dependencyManagerRootExtension { javaVersion.set(JavaVersion.VERSION_21) }.
	javaVersionSetRegex                = regexp.MustCompile(`\bjavaVersion\.set\(\s*` + javaVersionValuePattern + `\s*\)`)
	javaVersionSetFactoryPropertyRegex = regexp.MustCompile(`\bjavaVersion\.set\(\s*JavaLanguageVersion\.of\(\s*` + gradlePropertyPattern + `\s*\)\s*\)`)
	javaVersionSetPropertyRegex        = regexp.MustCompile(`\bjavaVersion\.set\(\s*` + gradlePropertyPattern + `\s*\)`)

	// Dynamic logic patterns that should trigger "unknown".
	dynamicPatterns = []*regexp.Regexp{
		regexp.MustCompile(`findProperty\s*\(`),
		regexp.MustCompile(`getProperty\s*\(`),
		regexp.MustCompile(`System\.getenv\s*\(`),
		regexp.MustCompile(`project\.properties\s*\[`),
		regexp.MustCompile(`extra\s*\[`),
		regexp.MustCompile(`ext\s*\[`),
		regexp.MustCompile(`val\s+\w+\s*=.*JavaVersion`),
		regexp.MustCompile(`def\s+\w+\s*=.*JavaVersion`),
	}
)

// detectGradle detects Java version from Gradle build files.
// Precedence (strict order):
//  1. sourceCompatibility / targetCompatibility (must match if both present)
//  2. kotlinOptions.jvmTarget (best-effort; used only if source/target are absent)
//  3. java.toolchain.languageVersion
//  4. javaVersion assignment (e.g. dependencyManagerRootExtension.javaVersion)
//  5. gradle/libs.versions.toml [versions].jvmTarget or [versions].jdk
func detectGradle(ctx context.Context, workspace, gradlePath string) (*Observation, error) {
	content, err := os.ReadFile(gradlePath)
	if err != nil {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "failed to read build.gradle: " + err.Error(),
		}
	}

	text := string(content)
	relativePath := relPath(workspace, gradlePath)
	propertyResolver := gradlePropertiesResolver{workspace: workspace}

	// 1. Check sourceCompatibility and targetCompatibility.
	sourceVersion := extractCompatibilityVersion(sourceCompatibilityRegex, text)
	targetVersion := extractCompatibilityVersion(targetCompatibilityRegex, text)

	if observation, err := resolveGradleVersionCandidates(
		literalGradleVersionCandidate(sourceVersion, relativePath, "sourceCompatibility"),
		literalGradleVersionCandidate(targetVersion, relativePath, "targetCompatibility"),
		"sourceCompatibility and targetCompatibility differ",
	); observation != nil || err != nil {
		return observation, err
	}

	// 2. Kotlin JVM hint: kotlinOptions.jvmTarget.
	jvmTargetDirect := extractCompatibilityVersion(kotlinOptionsJvmTargetDirectRegex, text)
	jvmTargetBlock := extractCompatibilityVersion(kotlinOptionsJvmTargetBlockRegex, text)

	if observation, err := resolveGradleVersionCandidates(
		literalGradleVersionCandidate(jvmTargetDirect, relativePath, "kotlinOptions.jvmTarget"),
		literalGradleVersionCandidate(jvmTargetBlock, relativePath, "kotlinOptions.jvmTarget"),
		"kotlinOptions.jvmTarget differs between assignments",
	); observation != nil || err != nil {
		return observation, err
	}

	// 3. Java toolchain languageVersion.
	toolchainAssignRef := extractGradleVersionRef(
		toolchainLanguageVersionAssignRegex,
		text,
		toolchainLanguageVersionAssignPropertyRegex,
	)
	toolchainSetRef := extractGradleVersionRef(
		toolchainLanguageVersionSetRegex,
		text,
		toolchainLanguageVersionSetFactoryPropertyRegex,
		toolchainLanguageVersionSetPropertyRegex,
	)
	toolchainVersionAssign, toolchainAssignEvidence, err := resolveGradleVersionRef(
		&propertyResolver, toolchainAssignRef, relativePath, "java.toolchain.languageVersion",
	)
	if err != nil {
		return nil, err
	}
	toolchainVersionSet, toolchainSetEvidence, err := resolveGradleVersionRef(
		&propertyResolver, toolchainSetRef, relativePath, "java.toolchain.languageVersion",
	)
	if err != nil {
		return nil, err
	}
	if observation, err := resolveGradleVersionCandidates(
		gradleVersionCandidate{value: toolchainVersionAssign, evidence: toolchainAssignEvidence},
		gradleVersionCandidate{value: toolchainVersionSet, evidence: toolchainSetEvidence},
		"toolchain languageVersion differs between assignments",
	); observation != nil || err != nil {
		return observation, err
	}

	// 4. Generic javaVersion assignment often used by custom Gradle extensions.
	javaVersionAssignmentRef := extractGradleVersionRef(
		javaVersionAssignmentRegex,
		text,
		javaVersionAssignmentFactoryPropertyRegex,
		javaVersionAssignmentPropertyRegex,
	)
	javaVersionSetRef := extractGradleVersionRef(
		javaVersionSetRegex,
		text,
		javaVersionSetFactoryPropertyRegex,
		javaVersionSetPropertyRegex,
	)
	javaVersionAssignment, javaVersionAssignmentEvidence, err := resolveGradleVersionRef(
		&propertyResolver, javaVersionAssignmentRef, relativePath, "javaVersion",
	)
	if err != nil {
		return nil, err
	}
	javaVersionSet, javaVersionSetEvidence, err := resolveGradleVersionRef(
		&propertyResolver, javaVersionSetRef, relativePath, "javaVersion",
	)
	if err != nil {
		return nil, err
	}
	if observation, err := resolveGradleVersionCandidates(
		gradleVersionCandidate{value: javaVersionAssignment, evidence: javaVersionAssignmentEvidence},
		gradleVersionCandidate{value: javaVersionSet, evidence: javaVersionSetEvidence},
		"javaVersion differs between assignments",
	); observation != nil || err != nil {
		return observation, err
	}

	// 5. Version catalog Java release.
	if observation, err := detectGradleVersionCatalogJavaVersion(workspace); observation != nil || err != nil {
		return observation, err
	}

	// No Java version found.
	for _, pattern := range dynamicPatterns {
		if pattern.MatchString(text) {
			return nil, &DetectionError{
				Reason:  "unknown",
				Message: "build.gradle contains dynamic version logic; cannot reliably detect Java version",
			}
		}
	}
	return nil, &DetectionError{
		Reason:  "unknown",
		Message: "no supported Java version configuration found in " + filepath.Base(gradlePath),
	}
}

type gradleVersionRef struct {
	value    string
	property string
}

type gradleVersionCandidate struct {
	value    string
	evidence []EvidenceItem
}

func literalGradleVersionCandidate(value, path, key string) gradleVersionCandidate {
	return gradleVersionCandidate{
		value:    value,
		evidence: []EvidenceItem{{Path: path, Key: key, Value: value}},
	}
}

func resolveGradleVersionCandidates(first, second gradleVersionCandidate, conflictMessage string) (*Observation, error) {
	if first.value == "" && second.value == "" {
		return nil, nil
	}

	evidence := make([]EvidenceItem, 0, len(first.evidence)+len(second.evidence))
	if first.value != "" {
		evidence = append(evidence, first.evidence...)
	}
	if second.value != "" {
		evidence = append(evidence, second.evidence...)
	}
	if first.value != "" && second.value != "" && first.value != second.value {
		return nil, &DetectionError{
			Reason:   "unknown",
			Message:  conflictMessage,
			Evidence: evidence,
		}
	}

	version := first.value
	if version == "" {
		version = second.value
	}
	return &Observation{
		Language: "java",
		Tool:     "gradle",
		Release:  &version,
		Evidence: evidence,
	}, nil
}

func extractGradleVersionRef(literalRegex *regexp.Regexp, text string, propertyRegexes ...*regexp.Regexp) gradleVersionRef {
	if value := extractCompatibilityVersion(literalRegex, text); value != "" {
		return gradleVersionRef{value: value}
	}
	for _, propertyRegex := range propertyRegexes {
		matches := propertyRegex.FindStringSubmatch(text)
		if len(matches) >= 2 && matches[1] != "" {
			return gradleVersionRef{property: matches[1]}
		}
	}
	return gradleVersionRef{}
}

func resolveGradleVersionRef(
	resolver *gradlePropertiesResolver,
	ref gradleVersionRef,
	buildPath string,
	buildKey string,
) (string, []EvidenceItem, error) {
	value, propertyEvidence, err := resolver.resolve(ref)
	if err != nil || value == "" {
		return value, nil, err
	}

	evidence := []EvidenceItem{{Path: buildPath, Key: buildKey, Value: value}}
	if propertyEvidence != nil {
		evidence = append(evidence, *propertyEvidence)
	}
	return value, evidence, nil
}

type gradleVersionCatalog struct {
	Versions struct {
		JVMTarget string `toml:"jvmTarget"`
		JDK       string `toml:"jdk"`
	} `toml:"versions"`
}

func detectGradleVersionCatalogJavaVersion(workspace string) (*Observation, error) {
	const rel = "gradle/libs.versions.toml"
	path := filepath.Join(workspace, rel)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "failed to read " + rel + ": " + err.Error(),
		}
	}

	var catalog gradleVersionCatalog
	if err := toml.Unmarshal(content, &catalog); err != nil {
		return nil, &DetectionError{
			Reason:  "unknown",
			Message: "failed to parse " + rel + ": " + err.Error(),
		}
	}

	return resolveGradleVersionCandidates(
		literalGradleVersionCandidate(
			normalizeJavaVersion(catalog.Versions.JVMTarget), rel, "versions.jvmTarget",
		),
		literalGradleVersionCandidate(
			normalizeJavaVersion(catalog.Versions.JDK), rel, "versions.jdk",
		),
		"version catalog jvmTarget and jdk differ",
	)
}

// extractCompatibilityVersion extracts version from sourceCompatibility or targetCompatibility patterns.
// Returns the numeric version string or empty if not found.
func extractCompatibilityVersion(regex *regexp.Regexp, text string) string {
	matches := regex.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	for i := 1; i < len(matches); i++ {
		if matches[i] == "" {
			continue
		}
		return normalizeJavaVersion(matches[i])
	}
	return ""
}

// normalizeJavaVersion normalizes legacy version strings like "1.8" to "8".
func normalizeJavaVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "_", ".")
	if strings.HasPrefix(v, "1.") {
		if n, err := strconv.Atoi(strings.TrimPrefix(v, "1.")); err == nil {
			return strconv.Itoa(n)
		}
	}
	return v
}
