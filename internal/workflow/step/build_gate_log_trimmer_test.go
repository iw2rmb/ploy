package step

import (
	"strings"
	"testing"
)

func TestTrimBuildGateLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		tool         string
		logText      string
		wantTrimmed  bool   // true if output should differ from input
		wantPrefix   string // expected prefix of trimmed output (after TrimSpace)
		wantContains []string
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
			name: "gradle with failure header",
			tool: "gradle",
			logText: `
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
`,
			wantTrimmed: true,
			wantContains: []string{
				"FAILURE: Build failed with an exception.",
				"Execution failed for task ':sample:test'.",
				"BUILD FAILED in",
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
			if tt.wantPrefix != "" && !strings.HasPrefix(strings.TrimSpace(trimmed), tt.wantPrefix) {
				t.Errorf("trimmed log should start with %q, got:\n%s", tt.wantPrefix, trimmed)
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(trimmed, s) {
					t.Errorf("trimmed log missing %q", s)
				}
			}
		})
	}
}
