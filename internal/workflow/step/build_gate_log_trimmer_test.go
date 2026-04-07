package step

import (
	"strings"
	"testing"
)

func TestTrimBuildGateLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tool            string
		logText         string
		wantTrimmed     bool   // true if output should differ from input
		wantPrefix      string // expected prefix of trimmed output (after TrimSpace)
		wantContains    []string
		wantNotContains []string
		wantExact       string
	}{
		{
			name: "maven with error summary",
			tool: "maven",
			logText: `
[INFO] --- maven-surefire-plugin:3.2.5:test (default-test) @ sample ---
[INFO]
[INFO] -------------------------------------------------------
[INFO]  T E S T S
[INFO] -------------------------------------------------------
[INFO] Running ru.tbank.tpe.ignite.client.pooled.PublishEventsTest
2025-11-27 04:54:20.749  INFO 198 --- [           main] .t.t.i.c.c.DefaultCustomizersEnabledTest : Starting
2025-11-27 04:54:20.896  INFO 198 --- [           main] trationDelegate$BeanPostProcessorChecker : Bean 'ru.tbank.tpe.ignite.client.mock.MocksConfig'
org.apache.ignite.client.ClientConnectionException: Connection refused
	at java.base/java.net.PlainSocketImpl.socketConnect(Native Method)
	at java.base/java.net.AbstractPlainSocketImpl.doConnect(AbstractPlainSocketImpl.java:412)
Caused by: java.net.ConnectException: Connection refused
	at java.base/sun.nio.ch.Net.pollConnect(Native Method)
	at java.base/sun.nio.ch.Net.pollConnectNow(Net.java:672)
[ERROR] Tests run: 1, Failures: 0, Errors: 1, Skipped: 0, Time elapsed: 0.855 s <<< FAILURE! -- in ru.tbank.tpe.ignite.client.pooled.PublishEventsTest
[ERROR] ru.tbank.tpe.ignite.client.pooled.PublishEventsTest.sendNewNewIgniteConfigurationEventTest -- Time elapsed: 0.025 s <<< ERROR!
org.springframework.beans.factory.BeanCreationException:
Error creating bean with name 'pooledLazyIgniteClient'
	at org.springframework.beans.factory.support.AbstractAutowireCapableBeanFactory.doCreateBean(AbstractAutowireCapableBeanFactory.java:628)
Caused by: org.mockito.exceptions.base.MockitoException:
Cannot mock/spy class ru.tbank.tpe.ignite.client.pooled.IgnitePooledClientAutoconfiguration$NoopPooledLazyIgniteClient
Mockito cannot mock/spy because :
 - final class
[INFO] BUILD FAILURE
[INFO] Total time:  23.456 s
`,
			wantTrimmed:  true,
			wantPrefix:   "[ERROR] Tests run:",
			wantContains: []string{"[ERROR] Tests run:", "Cannot mock/spy class"},
		},
		{
			name: "gradle compile issues keeps latest K, removes try, dedupes stack and preserves failure",
			tool: "gradle",
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
				"* Exception is:",
				"...repeated 2 frames",
				"BUILD FAILED in",
			},
			wantNotContains: []string{
				"/workspace/src/main/java/a/Old.java:1: error: cannot find symbol",
				"* Try:",
				"> Run with --info or --debug option to get more log output.",
			},
		},
		{
			name: "gradle dedupes tab-prefixed stacktrace frames",
			tool: "gradle",
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
				"...repeated 3 frames",
				"BUILD FAILED in 1s",
			},
			wantNotContains: []string{
				"\tat org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.executeIfValid(ExecuteActionsTaskExecuter.java:142)\n\t\tat org.gradle.api.internal.tasks.execution.ExecuteActionsTaskExecuter.execute(ExecuteActionsTaskExecuter.java:131)\n\t\tat org.gradle.other.Frame.y(Frame.java:1)",
			},
		},
		{
			name: "gradle kotlin scala and kapt issue collection",
			tool: "gradle",
			logText: `
e: /workspace/src/main/kotlin/a/App.kt:7:11 Unresolved reference: Foo
   ^
[error] /workspace/src/main/scala/a/App.scala:9: not found: type Foo
[error]  val x: Foo = ??? 
e: [kapt] An exception occurred: java.lang.IllegalStateException: kapt blew up
  at org.jetbrains.kotlin.kapt3.base.AnnotationProcessingKt.doAnnotationProcessing(annotationProcessing.kt:90)
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
		},
		{
			name: "gradle without what went wrong passthrough",
			tool: "gradle",
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
			name: "gradle preserves trailing newline",
			tool: "gradle",
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
		},
		{
			name:        "unknown tool passthrough",
			tool:        "unknown",
			logText:     "some tool output\nwith multiple lines\n",
			wantTrimmed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			trimmed := TrimBuildGateLog(tt.tool, tt.logText)

			if tt.wantTrimmed && trimmed == tt.logText {
				t.Fatal("expected log to be trimmed, but got original")
			}
			if !tt.wantTrimmed && trimmed != tt.logText {
				t.Fatal("expected original log, but got trimmed output")
			}
			if tt.wantExact != "" && trimmed != tt.wantExact {
				t.Errorf("unexpected exact output:\nwant:\n%s\ngot:\n%s", tt.wantExact, trimmed)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(strings.TrimSpace(trimmed), tt.wantPrefix) {
				t.Errorf("trimmed log should start with %q, got:\n%s", tt.wantPrefix, trimmed)
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(trimmed, s) {
					t.Errorf("trimmed log missing %q", s)
				}
			}
			for _, s := range tt.wantNotContains {
				if strings.Contains(trimmed, s) {
					t.Errorf("trimmed log should not contain %q", s)
				}
			}
			if strings.HasSuffix(tt.logText, "\n") != strings.HasSuffix(trimmed, "\n") {
				t.Errorf("trailing newline mismatch: input=%t output=%t", strings.HasSuffix(tt.logText, "\n"), strings.HasSuffix(trimmed, "\n"))
			}
		})
	}
}
