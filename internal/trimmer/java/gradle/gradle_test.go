package gradle

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTrim(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		logText         string
		wantTrimmed     bool
		wantPrefix      string
		wantContains    []string
		wantNotContains []string
		wantExact       string
		wantEvidence    bool
	}{
		{
			name: "compile issues keeps latest K removes try and exception stack",
			logText: `
/workspace/src/main/java/a/Old.java:1: error: cannot find symbol
  symbol:   class MissingA
/workspace/src/main/java/b/Old2.java:2: error: cannot find symbol
  symbol:   class MissingB
/workspace/src/main/java/c/New.java:3: error: cannot find symbol
  symbol:   class MissingC
2 errors
* What went wrong:
Execution failed for task ':compileJava'.
> Compilation failed; see the compiler error output for details.
* Try:
> Run with --info or --debug option to get more log output.
> Run with --scan to get full insights.
* Exception is:
  at org.example.Top.a(Top.java:1)
  at org.example.Top.b(Top.java:2)
Caused by: java.lang.RuntimeException: x
  at org.example.Top.b(Top.java:2)
  at org.example.Top.c(Top.java:3)
BUILD FAILED in 5s
`,
			wantTrimmed: true,
			wantPrefix:  "/workspace/src/main/java/b/Old2.java:2: error: cannot find symbol",
			wantContains: []string{
				"/workspace/src/main/java/c/New.java:3: error: cannot find symbol",
				"* What went wrong:",
				"Execution failed for task ':compileJava'.",
				"BUILD FAILED in",
			},
			wantNotContains: []string{
				"/workspace/src/main/java/a/Old.java:1: error: cannot find symbol",
				"* Try:",
				"* Exception is:",
				"...repeated",
				"> Run with --info or --debug option to get more log output.",
			},
			wantEvidence: true,
		},
		{
			name: "removes fallback exception stack",
			logText: `
* What went wrong:
Execution failed for task ':compileJava'.
> Compilation failed; see the compiler error output for details.
* Exception is:
	org.gradle.api.tasks.TaskExecutionException: Execution failed for task ':compileJava'.
		at org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.executeIfValid(ExecuteActionsTaskExecuter.java:142)
		at org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.execute(ExecuteActionsTaskExecuter.java:131)
Caused by: java.lang.RuntimeException: x
		at org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.executeIfValid(ExecuteActionsTaskExecuter.java:142)
		at org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.execute(ExecuteActionsTaskExecuter.java:131)
		at org.gradle.other.Frame.y(Frame.java:1)
BUILD FAILED in 1s
`,
			wantTrimmed: true,
			wantContains: []string{
				"* What went wrong:",
				"BUILD FAILED in 1s",
			},
			wantNotContains: []string{
				"* Exception is:",
				"TaskExecutionException",
				"...repeated",
			},
			wantEvidence: true,
		},
		{
			name: "kotlin scala and kapt issue collection",
			logText: `
e: /workspace/src/main/kotlin/a/App.kt:7:11 Unresolved reference: Foo
   ^
[error] /workspace/src/main/scala/a/App.scala:9: not found: type Foo
[error]  val x: Foo = ???
e: [kapt] An exception occurred: java.lang.IllegalStateException: kapt blew up
  at org.jetbrains.kotlin.kapt3.base.AnnotationProcessing(annotationProcessing.kt:90)
* What went wrong:
Execution failed for task ':compileKotlin'.
> Compilation error. See log for more details
BUILD FAILED in 1m
`,
			wantTrimmed: true,
			wantPrefix:  "e: /workspace/src/main/kotlin/a/App.kt:7:11 Unresolved reference: Foo",
			wantContains: []string{
				"[error] /workspace/src/main/scala/a/App.scala:9: not found: type Foo",
				"e: [kapt] An exception occurred: java.lang.IllegalStateException: kapt blew up",
				"* What went wrong:",
			},
			wantEvidence: true,
		},
		{
			name: "kotlin file uri issue collection",
			logText: `
e: file:///workspace/src/main/kotlin/a/App.kt:7:11 Unresolved reference: Foo
   ^
* What went wrong:
Execution failed for task ':compileKotlin'.
> Compilation error. See log for more details
BUILD FAILED in 1m
`,
			wantTrimmed: true,
			wantPrefix:  "e: file:///workspace/src/main/kotlin/a/App.kt:7:11 Unresolved reference: Foo",
			wantContains: []string{
				"* What went wrong:",
				"Execution failed for task ':compileKotlin'.",
			},
			wantEvidence: true,
		},
		{
			name: "kotlin compiler failure removes exception stack",
			logText: `
e: /workspace/src/main/kotlin/a/App.kt:7:11 Unresolved reference: Foo
   val value = Foo()
               ^
* What went wrong:
Execution failed for task ':compileKotlin'.
> Compilation error. See log for more details
* Exception is:
org.gradle.api.tasks.TaskExecutionException: Execution failed for task ':compileKotlin'.
  at org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.execute(ExecuteActionsTaskExecuter.java:131)
Caused by: org.jetbrains.kotlin.gradle.tasks.CompilationErrorException: Compilation error. See log for more details
  at org.jetbrains.kotlin.gradle.tasks.TasksUtilsKt.throwExceptionIfCompilationFailed(tasksUtils.kt:20)
BUILD FAILED in 2s
`,
			wantTrimmed: true,
			wantContains: []string{
				"e: /workspace/src/main/kotlin/a/App.kt:7:11 Unresolved reference: Foo",
				"* What went wrong:",
				"Execution failed for task ':compileKotlin'.",
				"BUILD FAILED in 2s",
			},
			wantNotContains: []string{
				"* Exception is:",
				"TaskExecutionException",
				"CompilationErrorException",
			},
			wantEvidence: true,
		},
		{
			name: "removes gradle deprecation footer but keeps build failed",
			logText: `
* What went wrong:
Execution failed for task ':compileKotlin'.
> Compilation error. See log for more details
Deprecated Gradle features were used in this build, making it incompatible with Gradle 9.0.

You can use '--warning-mode all' to show the individual deprecation warnings and determine if they come from your own scripts or plugins.

For more on this, please refer to https://docs.gradle.org/8.6/userguide/command_line_interface.html#sec:command_line_warnings in the Gradle documentation.

BUILD FAILED in 3s
2 actionable tasks: 2 executed
`,
			wantTrimmed: true,
			wantContains: []string{
				"Execution failed for task ':compileKotlin'.",
				"BUILD FAILED in 3s",
				"2 actionable tasks: 2 executed",
			},
			wantNotContains: []string{
				"Deprecated Gradle features were used",
				"--warning-mode all",
				"command_line_warnings",
			},
			wantEvidence: true,
		},
		{
			name: "truncates public ci cleanup tail after terminal failure",
			logText: `
* What went wrong:
Execution failed for task ':compileKotlin'.
> Compilation error. See log for more details
BUILD FAILED in 4s
1 actionable task: 1 executed
Post job cleanup.
/usr/bin/git config --global --add safe.directory /home/runner/work/project/project
Post job cleanup.
Cache hit occurred on the primary key gradle-linux
Uploading artifacts for failed job
`,
			wantTrimmed: true,
			wantContains: []string{
				"BUILD FAILED in 4s",
				"1 actionable task: 1 executed",
			},
			wantNotContains: []string{
				"Post job cleanup",
				"git config",
				"Cache hit",
				"Uploading artifacts",
			},
			wantEvidence: true,
		},
		{
			name: "removes interleaved gradle progress noise from publish failure",
			logText: `
FAILURE: Build failed with an exception.
* What went wrong:
Execution failed for task ':rewrite-core:publishNebulaPublicationToSonatypeRepository'.
Cached resource https://repo.maven.apache.org/maven2/org/jline/jline-terminal/3.27.1/jline-terminal-3.27.1.jar is up-to-date.
> Failed to publish publication 'nebula' to repository 'sonatype'
Build cache key for task ':rewrite-scala:compileScala' is 5fee89845e822ad052eae6f65efc4731
   > Could not GET 'https://central.sonatype.com/repository/maven-snapshots/org/openrewrite/rewrite-core/8.84.0-SNAPSHOT/maven-metadata.xml'. Received status code 403 from server: Forbidden
* Try:
Cached resource https://repo.maven.apache.org/maven2/org/jline/jline/3.27.1/jline-3.27.1.jar is up-to-date.
> Run with --debug option to get more log output.
Skipping task ':rewrite-scala:compileScala' as it is up-to-date.
> Get more help at https://help.gradle.org.
* Exception is:
> Task :rewrite-scala:compileScala UP-TO-DATE
org.gradle.api.tasks.TaskExecutionException: Execution failed for task ':rewrite-core:publishNebulaPublicationToSonatypeRepository'.
  at org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.execute(ExecuteActionsTaskExecuter.java:121)
gradle/actions: Writing build results to /home/runner/work/_temp/.gradle-actions/build-results/result.json
Caused by: org.gradle.internal.resource.transport.http.HttpErrorStatusCodeException: Could not GET 'https://central.sonatype.com/repository/maven-snapshots/org/openrewrite/rewrite-core/8.84.0-SNAPSHOT/maven-metadata.xml'. Received status code 403 from server: Forbidden
  at org.gradle.internal.resource.transport.http.ApacheCommonsHttpClient.processResponse(ApacheCommonsHttpClient.java:310)
BUILD FAILED in 1m 2s
87 actionable tasks: 25 executed, 62 up-to-date
gradle/actions: Writing build results to /home/runner/work/_temp/.gradle-actions/build-scans/result.json
`,
			wantTrimmed: true,
			wantContains: []string{
				"Execution failed for task ':rewrite-core:publishNebulaPublicationToSonatypeRepository'.",
				"> Failed to publish publication 'nebula' to repository 'sonatype'",
				"Received status code 403 from server: Forbidden",
				"BUILD FAILED in 1m 2s",
				"87 actionable tasks: 25 executed, 62 up-to-date",
			},
			wantNotContains: []string{
				"Cached resource",
				"Build cache key",
				"Skipping task",
				"* Try:",
				"* Exception is:",
				"TaskExecutionException",
				"gradle/actions",
			},
			wantEvidence: true,
		},
		{
			name: "without what went wrong returns bounded passthrough without evidence",
			logText: `
> Task :compileJava
FAILURE: Build failed with an exception.
BUILD FAILED in 2s
`,
			wantTrimmed: false,
			wantExact: `
> Task :compileJava
FAILURE: Build failed with an exception.
BUILD FAILED in 2s
`,
		},
		{
			name: "preserves trailing newline",
			logText: "/workspace/src/main/java/a/A.java:1: error: cannot find symbol\n" +
				"  symbol: class X\n" +
				"1 errors\n" +
				"* What went wrong:\n" +
				"Execution failed for task ':compileJava'.\n" +
				"* Try:\n" +
				"> help\n" +
				"BUILD FAILED in 1s\n",
			wantTrimmed: true,
			wantContains: []string{
				"Execution failed for task ':compileJava'.",
			},
			wantEvidence: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			message, evidence := trimGradleLogAndEvidence(tt.logText)

			if tt.wantTrimmed && message == tt.logText {
				t.Fatal("expected log to be trimmed, but got original")
			}
			if !tt.wantTrimmed && message != tt.logText {
				t.Fatal("expected original log, but got trimmed output")
			}
			if tt.wantExact != "" && message != tt.wantExact {
				t.Errorf("unexpected exact output:\nwant:\n%s\ngot:\n%s", tt.wantExact, message)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(strings.TrimSpace(message), tt.wantPrefix) {
				t.Errorf("trimmed log should start with %q, got:\n%s", tt.wantPrefix, message)
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(message, s) {
					t.Errorf("trimmed log missing %q", s)
				}
			}
			for _, s := range tt.wantNotContains {
				if strings.Contains(message, s) {
					t.Errorf("trimmed log should not contain %q", s)
				}
			}
			if strings.HasSuffix(tt.logText, "\n") != strings.HasSuffix(message, "\n") {
				t.Errorf("trailing newline mismatch: input=%t output=%t", strings.HasSuffix(tt.logText, "\n"), strings.HasSuffix(message, "\n"))
			}
			if gotEvidence := evidence != nil; gotEvidence != tt.wantEvidence {
				t.Fatalf("evidence present = %v, want %v", gotEvidence, tt.wantEvidence)
			}
		})
	}
}

func TestTrimEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		logText           string
		wantEvidenceParts []string
		wantNoEvidence    []string
		wantNoStacktrace  bool
	}{
		{
			name: "compile_java evidence with grouped file refs",
			logText: `
/workspace/src/main/java/a/A.java:10: error: cannot find symbol
  symbol:   class Missing
/workspace/src/main/java/b/B.java:20: error: cannot find symbol
  symbol:   class Missing
2 errors
* What went wrong:
Execution failed for task ':compileJava'.
> Compilation failed; see the compiler error output for details.
* Exception is:
org.gradle.api.tasks.TaskExecutionException: Execution failed for task ':compileJava'.
  at org.example.Top.a(Top.java:1)
BUILD FAILED in 5s
`,
			wantEvidenceParts: []string{
				"task: compileJava",
				"symbol: class Missing",
				"base: /workspace/src/main/java/",
				"- a/A.java:10",
				"- b/B.java:20",
			},
			wantNoEvidence:   []string{"line:"},
			wantNoStacktrace: true,
		},
		{
			name: "subproject compile_java evidence with grouped file refs",
			logText: `
/workspace/src/main/java/a/A.java:10: error: cannot find symbol
  symbol:   class Missing
/workspace/src/main/java/b/B.java:20: error: cannot find symbol
  symbol:   class Missing
2 errors
* What went wrong:
Execution failed for task ':some:module:compileJava'.
> Compilation failed; see the compiler error output for details.
BUILD FAILED in 5s
`,
			wantEvidenceParts: []string{
				"task: compileJava",
				"symbol: class Missing",
				"base: /workspace/src/main/java/",
				"- a/A.java:10",
				"- b/B.java:20",
			},
			wantNoEvidence: []string{"task: some:module:compileJava", "line:"},
		},
		{
			name: "compile_java hoists common snippet and normalizes single-file error",
			logText: `
/workspace/src/main/java/a/A.java:10: error: cannot find symbol
  return Mono.subscriberContext()
         ^
  symbol:   method subscriberContext()
1 errors
* What went wrong:
Execution failed for task ':compileJava'.
> Compilation failed; see the compiler error output for details.
BUILD FAILED in 5s
`,
			wantEvidenceParts: []string{
				"base: /workspace/src/main/java/a/",
				"snippet: return Mono.subscriberContext()",
				"- A.java:10",
			},
			wantNoEvidence: []string{"line:", "path:", "files:\n    - path: A.java:10\n      snippet:"},
		},
		{
			name: "plugin_apply evidence includes plugin id and version",
			logText: `
FAILURE: Build failed with an exception.
* What went wrong:
An exception occurred applying plugin request [id: 'org.springframework.boot', version: '3.0.5']
> Failed to apply plugin 'org.springframework.boot'.
   > Spring Boot plugin requires Gradle 7.x (7.4 or later).
* Try:
> Run with --stacktrace option to get the stack trace.
BUILD FAILED in 27s
`,
			wantEvidenceParts: []string{"plugin: org.springframework.boot", "version: 3.0.5", "Spring Boot plugin requires Gradle 7.x"},
			wantNoEvidence:    []string{"plugin_id:", "plugin_version:", "Failed to apply plugin"},
		},
		{
			name: "compile_kotlin evidence with grouped file refs",
			logText: `
e: /workspace/src/main/kotlin/a/A.kt:10:15 Unresolved reference: Missing
   val value = Missing()
               ^
e: file:///workspace/src/main/kotlin/b/B.kt:20:21 Unresolved reference: Missing
   val value = Missing()
               ^
2 errors
* What went wrong:
Execution failed for task ':compileKotlin'.
> Compilation error. See log for more details
BUILD FAILED in 5s
`,
			wantEvidenceParts: []string{
				"task: compileKotlin",
				"message: 'Unresolved reference: Missing'",
				"base: /workspace/src/main/kotlin/",
				"snippet: val value = Missing()",
				"- a/A.kt:10:15",
				"- b/B.kt:20:21",
			},
			wantNoEvidence: []string{"line:", "path:", "file:///workspace"},
		},
		{
			name: "subproject compile_kotlin evidence uses terminal task name",
			logText: `
e: /workspace/module/src/main/kotlin/App.kt:7:11 Unresolved reference: Foo
   Foo()
   ^
* What went wrong:
Execution failed for task ':some:module:compileKotlin'.
> Compilation error. See log for more details
BUILD FAILED in 1s
`,
			wantEvidenceParts: []string{
				"task: compileKotlin",
				"message: 'Unresolved reference: Foo'",
				"base: /workspace/module/src/main/kotlin/",
				"- App.kt:7:11",
			},
			wantNoEvidence: []string{"task: some:module:compileKotlin", "line:", "path:"},
		},
		{
			name: "kapt_kotlin evidence uses terminal task name",
			logText: `
e: /workspace/src/main/kotlin/App.kt:12:5 Unresolved reference: GeneratedType
   GeneratedType()
   ^
* What went wrong:
Execution failed for task ':kaptKotlin'.
> Compilation error. See log for more details
BUILD FAILED in 1s
`,
			wantEvidenceParts: []string{
				"task: kaptKotlin",
				"message: 'Unresolved reference: GeneratedType'",
				"- App.kt:12:5",
			},
			wantNoEvidence: []string{"line:", "path:"},
		},
		{
			name: "raw evidence fallback",
			logText: `
* What went wrong:
Execution failed for task ':customTask'.
> Process 'command 'bash'' finished with non-zero exit value 1
BUILD FAILED in 1s
`,
			wantEvidenceParts: []string{"customTask", "non-zero exit value 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Trim(tt.logText)
			if result.Evidence == nil {
				t.Fatal("expected evidence payload")
			}
			evidence, err := yaml.Marshal(result.Evidence)
			if err != nil {
				t.Fatalf("marshal evidence: %v", err)
			}
			payload := string(evidence)

			var decoded map[string]any
			if err := yaml.Unmarshal(evidence, &decoded); err != nil {
				t.Fatalf("unmarshal evidence: %v\npayload:\n%s", err, payload)
			}
			if _, hasMode := decoded["mode"]; hasMode {
				t.Fatalf("unexpected mode discriminator in payload:\n%s", payload)
			}
			for _, part := range tt.wantEvidenceParts {
				if !strings.Contains(payload, part) {
					t.Errorf("evidence missing %q", part)
				}
			}
			for _, part := range tt.wantNoEvidence {
				if strings.Contains(payload, part) {
					t.Errorf("evidence should not contain %q", part)
				}
			}
			if tt.wantNoStacktrace && strings.Contains(payload, "TaskExecutionException") {
				t.Fatalf("evidence should exclude stacktrace, got:\n%s", payload)
			}
		})
	}
}

func TestTrimMessageEmission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		logText     string
		wantMessage bool
	}{
		{
			name: "complete compile_java evidence omits root message",
			logText: `
/workspace/src/main/java/a/A.java:10: error: cannot find symbol
  symbol:   class Missing
1 errors
* What went wrong:
Execution failed for task ':compileJava'.
> Compilation failed; see the compiler error output for details.
BUILD FAILED in 5s
`,
		},
		{
			name: "complete compile_kotlin evidence omits root message",
			logText: `
e: file:///workspace/src/main/kotlin/a/A.kt:10:15 Unresolved reference: Missing
   Missing()
   ^
* What went wrong:
Execution failed for task ':compileKotlin'.
> Compilation error. See log for more details
BUILD FAILED in 5s
`,
		},
		{
			name: "fallback evidence keeps root message",
			logText: `
* What went wrong:
Execution failed for task ':customTask'.
> Process 'command 'bash'' finished with non-zero exit value 1
BUILD FAILED in 1s
`,
			wantMessage: true,
		},
		{
			name: "unparsed log keeps root message",
			logText: `
> Task :compileJava
FAILURE: Build failed with an exception.
BUILD FAILED in 2s
`,
			wantMessage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Trim(tt.logText)
			if result.Tool != Tool {
				t.Fatalf("Tool = %q, want %q", result.Tool, Tool)
			}
			if got := strings.TrimSpace(result.Message) != ""; got != tt.wantMessage {
				t.Fatalf("message present = %v, want %v; message=%q", got, tt.wantMessage, result.Message)
			}
		})
	}
}

func TestTrimMessageBounds(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := 0; i < 260; i++ {
		b.WriteString(strings.Repeat("x", 400))
		b.WriteByte('\n')
	}

	result := Trim(b.String())
	if len(strings.Split(result.Message, "\n")) > maxMessageLines {
		t.Fatalf("message has too many lines")
	}
	if len(result.Message) > maxMessageBytes {
		t.Fatalf("message has %d bytes, want <= %d", len(result.Message), maxMessageBytes)
	}
}
