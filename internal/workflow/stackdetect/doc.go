// Package stackdetect provides filesystem-only, deterministic detection of
// technology stacks from build files.
//
// # Overview
//
// This package detects the programming language, build tool, and release version
// from a workspace by examining build files. Detection is purely filesystem-based
// (no execution of build tools) and produces deterministic results for a given
// workspace state.
//
// # Supported Stacks
//
// Currently supported:
//   - Java with Maven (pom.xml)
//   - Java with Gradle (build.gradle, build.gradle.kts)
//
// # Usage
//
//	obs, err := stackdetect.Detect(ctx, "/path/to/workspace")
//	if err != nil {
//	    var detErr *stackdetect.DetectionError
//	    if errors.As(err, &detErr) {
//	        if detErr.IsAmbiguous() {
//	            // Multiple build tools present
//	        } else if detErr.IsUnknown() {
//	            // No detection possible
//	        }
//	    }
//	    return err
//	}
//	fmt.Printf("Detected: %s %s %s\n", obs.Language, obs.Tool, *obs.Release)
//
// # Detection Logic
//
// Maven (pom.xml) precedence:
//  1. maven.compiler.release property
//  2. maven.compiler.source + maven.compiler.target (must match)
//  3. java.version property
//
// Gradle (build.gradle/.kts) precedence:
//  1. JavaLanguageVersion.of(N) - toolchain API
//  2. sourceCompatibility / targetCompatibility (must match if both present)
//
// # Error Handling
//
// Detection returns a DetectionError with reason codes:
//   - "ambiguous": Both Maven and Gradle build files present
//   - "unknown": No build files found, or version cannot be determined
//
// # Limitations
//
// This package performs static analysis only:
//   - Maven property interpolation is limited to local parent POMs
//   - Gradle detection uses regex patterns (no Groovy/Kotlin parsing)
//   - Dynamic version logic in Gradle (variables, findProperty, etc.) returns "unknown"
package stackdetect
