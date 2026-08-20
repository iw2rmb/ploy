package stackdetect

import "testing"

func TestNormalizeJavaVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.8", "8"}, {"1.7", "7"}, {"1.6", "6"}, {"1.5", "5"},
		{"8", "8"}, {"11", "11"}, {"17", "17"}, {"21", "21"}, {"  17  ", "17"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeJavaVersion(tt.input); got != tt.want {
				t.Fatalf("normalizeJavaVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGradleVersionRegexes(t *testing.T) {
	source := func(input string) string { return extractCompatibilityVersion(sourceCompatibilityRegex, input) }
	target := func(input string) string { return extractCompatibilityVersion(targetCompatibilityRegex, input) }
	kotlinDirect := func(input string) string {
		return extractCompatibilityVersion(kotlinOptionsJvmTargetDirectRegex, input)
	}
	kotlinBlock := func(input string) string { return extractCompatibilityVersion(kotlinOptionsJvmTargetBlockRegex, input) }
	javaVersion := func(input string) string { return extractCompatibilityVersion(javaVersionAssignmentRegex, input) }
	tests := []struct {
		name    string
		extract func(string) string
		input   string
		want    string
	}{
		{"source quoted", source, `sourceCompatibility = "17"`, "17"},
		{"source number", source, `sourceCompatibility = 11`, "11"},
		{"source qualified", source, `sourceCompatibility = JavaVersion.VERSION_17`, "17"},
		{"source unqualified", source, `sourceCompatibility = VERSION_21`, "21"},
		{"source legacy", source, `sourceCompatibility = JavaVersion.VERSION_1_8`, "8"},
		{"source spaces", source, `sourceCompatibility   =   21`, "21"},
		{"source no match", source, `targetCompatibility = 17`, ""},
		{"target quoted", target, `targetCompatibility = "17"`, "17"},
		{"target number", target, `targetCompatibility = 11`, "11"},
		{"target qualified", target, `targetCompatibility = JavaVersion.VERSION_17`, "17"},
		{"Kotlin direct quoted", kotlinDirect, `kotlinOptions.jvmTarget = "17"`, "17"},
		{"Kotlin direct constant", kotlinDirect, `kotlinOptions.jvmTarget = JavaVersion.VERSION_21`, "21"},
		{"Kotlin direct legacy", kotlinDirect, `kotlinOptions.jvmTarget = JavaVersion.VERSION_1_8`, "8"},
		{"Kotlin direct no match", kotlinDirect, `kotlinOptions { jvmTarget = "17" }`, ""},
		{"Kotlin block single line", kotlinBlock, `kotlinOptions { jvmTarget = "17" }`, "17"},
		{"Kotlin block multi line", kotlinBlock, "kotlinOptions {\n jvmTarget = JavaVersion.VERSION_21\n}", "21"},
		{"Kotlin block no match", kotlinBlock, `kotlinOptions.jvmTarget = "17"`, ""},
		{"javaVersion qualified", javaVersion, `javaVersion = JavaVersion.VERSION_21`, "21"},
		{"javaVersion unqualified", javaVersion, `javaVersion = VERSION_17`, "17"},
		{"javaVersion legacy", javaVersion, `javaVersion = JavaVersion.VERSION_1_8`, "8"},
		{"javaVersion number", javaVersion, `javaVersion = 11`, "11"},
		{"javaVersion factory", javaVersion, `javaVersion = JavaLanguageVersion.of("21")`, "21"},
		{"javaVersion no match", javaVersion, `sourceCompatibility = 17`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.extract(tt.input); got != tt.want {
				t.Fatalf("version = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDynamicPatterns(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"findProperty", `val javaVersion = findProperty("javaVersion")`, true},
		{"getProperty", `def version = getProperty("version")`, true},
		{"environment", `val jdk = System.getenv("JAVA_HOME")`, true},
		{"project properties", `val prop = project.properties["javaVersion"]`, true},
		{"extra property", `val ver = extra["javaVersion"]`, true},
		{"ext property", `def ver = ext["javaVersion"]`, true},
		{"conditional", `val javaVer = if (condition) JavaVersion.VERSION_17 else JavaVersion.VERSION_11`, true},
		{"static source", `sourceCompatibility = 17`, false},
		{"static Kotlin", `kotlinOptions.jvmTarget = "17"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := false
			for _, pattern := range dynamicPatterns {
				if pattern.MatchString(tt.input) {
					matched = true
					break
				}
			}
			if matched != tt.want {
				t.Fatalf("dynamic match = %v, want %v for %q", matched, tt.want, tt.input)
			}
		})
	}
}
