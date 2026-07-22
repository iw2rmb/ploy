• Extend it as a conservative static resolver, not by executing Gradle.

  The target pattern is:

  val projectJavaVersion: String by project

  java {
      toolchain {
          languageVersion.set(JavaLanguageVersion.of(projectJavaVersion))
      }
  }

  with:

  projectJavaVersion=25

  Recommended shape:

  1. Add a small gradle.properties reader in internal/workflow/stackdetect/gradle.go:67.

  Parse only local root gradle.properties, into map[string]string. Keep it strict and deterministic:

  - ignore blank lines and # / ! comments
  - split on first = or :
  - trim key/value
  - accept only values that normalize as Java versions, e.g. 25, "25", 1.8
  - do not resolve environment variables, findProperty, providers.gradleProperty, expressions, or computed values

  2. Teach version extraction to return “property reference” as well as literal value.

  Right now javaVersionValuePattern only captures literal numeric or JavaVersion.VERSION_* forms at internal/workflow/stackdetect/
  gradle.go:30. Add separate property-aware regexes for the limited forms:

  JavaLanguageVersion.of(projectJavaVersion)
  languageVersion.set(projectJavaVersion)
  javaVersion = projectJavaVersion
  javaVersion.set(projectJavaVersion)

  Do not make the main regex too clever. A helper like this is clearer:

  type gradleVersionRef struct {
      value string
      property string
  }

  Then resolve:

  - if value != "", use current behavior
  - if property != "", look it up in loaded gradle.properties
  - if missing or non-numeric, return no match and let existing unknown/dynamic handling continue

  3. Preserve precedence.

  Keep the current order in internal/workflow/stackdetect/gradle.go:60:

  1. sourceCompatibility / targetCompatibility
  2. kotlinOptions.jvmTarget
  3. java.toolchain.languageVersion
  4. javaVersion
  5. version catalog versions.jvmTarget

  Only property-resolution should be added inside steps 3 and 4. Do not let gradle.properties globally override explicit build-file
  literals.

  4. Add evidence that names both files.

  For the failing case, evidence should show the build-file key and resolved property source, for example:

  [
    {"path":"build.gradle.kts","key":"java.toolchain.languageVersion","value":"25"},
    {"path":"gradle.properties","key":"projectJavaVersion","value":"25"}
  ]

  That makes future run-status/debug output explain why Ploy chose 25.

  5. Add rows to the existing table-driven test, not a new test function.

  The matching test table is in internal/workflow/stackdetect/gradle_test.go:300. Add cases like:

  - toolchain languageVersion set from gradle property
  - toolchain JavaLanguageVersion factory from gradle property
  - dependency manager javaVersion from gradle property
  - missing property returns error
  - non-version property returns error

  The core positive case should look like:

  {
      name:     "toolchain languageVersion set from gradle property",
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
  }

  Important boundary: do not turn this into a Gradle interpreter. Support only direct property indirection from gradle.properties to
  known Java-version fields. That fixes this incident class while keeping stack detection deterministic and filesystem-only.
