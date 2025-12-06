package step

import (
	"strings"
	"testing"
)

func TestTrimBuildGateLog_Maven_WithErrorSummary(t *testing.T) {
	t.Parallel()

	const logText = `
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
`

	trimmed := TrimBuildGateLog("maven", logText)

	if trimmed == logText {
		t.Fatalf("TrimBuildGateLog(maven) did not trim logs")
	}

	// New behavior: Maven trimmer keeps everything starting from the first
	// "[ERROR]" line to the end of the log.
	if !strings.HasPrefix(strings.TrimSpace(trimmed), "[ERROR] Tests run:") {
		t.Errorf("trimmed log should start at first [ERROR] line, got:\n%s", trimmed)
	}

	if !strings.Contains(trimmed, "[ERROR] Tests run:") {
		t.Errorf("trimmed log missing error summary: %s", trimmed)
	}
	if !strings.Contains(trimmed, "Cannot mock/spy class") {
		t.Errorf("trimmed log missing stack trace snippet")
	}
	// Footer lines (e.g. BUILD FAILURE / Total time) may remain; we no longer
	// strip them when anchoring on the first [ERROR] line.
}

func TestTrimBuildGateLog_Gradle_WithFailureHeader(t *testing.T) {
	t.Parallel()

	const logText = `
> Task :compileJava
> Task :test

FAILURE: Build failed with an exception.

* What went wrong:
Execution failed for task ':sample:test'.
> There were failing tests. See the report at: file:///workspace/build/reports/tests/test/index.html

* Try:
> Run with --stacktrace option to get the stack trace.

BUILD FAILED in 5s
3 actionable tasks: 2 executed, 1 up-to-date
`

	trimmed := TrimBuildGateLog("gradle", logText)

	if trimmed == logText {
		t.Fatalf("TrimBuildGateLog(gradle) did not trim logs")
	}
	if !strings.Contains(trimmed, "FAILURE: Build failed with an exception.") {
		t.Errorf("trimmed log missing Gradle failure header: %s", trimmed)
	}
	if !strings.Contains(trimmed, "Execution failed for task ':sample:test'.") {
		t.Errorf("trimmed log missing task failure details")
	}
	if !strings.Contains(trimmed, "BUILD FAILED in") {
		t.Errorf("trimmed log should keep BUILD FAILED summary")
	}
	// Gradle trimmer also keeps a small amount of task context above the
	// failure header, so we do not assert on task lines being removed.
}

func TestTrimBuildGateLog_UnknownToolPassthrough(t *testing.T) {
	t.Parallel()

	const logText = "some tool output\nwith multiple lines\n"
	trimmed := TrimBuildGateLog("unknown", logText)
	if trimmed != logText {
		t.Fatalf("expected unknown tool to return original logs")
	}
}
