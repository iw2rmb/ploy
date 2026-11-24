# Reduce full Maven test runs during healing

In the Spring Boot 3.5 to 4.0 replay of `ms/cb-platform-gateway:f197631e`, the first Maven test run executed 2,025 tests. It reported 4 failures and 1,303 errors. The fourth run reported 8 failures and 128 errors. Later full runs took about seven minutes each. [Run evidence](https://ploy.t-tech.team/v1/runs/3MXYUrSX6JbDi1WWCIxhfeEPsNg/status)

In the Java migration repository, `healing/maven-tests.yaml` selects the first fresh failing Surefire or Failsafe XML report in path order. After the coder repairs that failure, `maven_tests_audit_cycle` starts the configured Maven goals again. One cycle cleared a single assertion in `OfferControllerTest` while 128 errors remained.

## Proposed experiment

Use all fresh reports from one Maven invocation. Group failures by the underlying exception and relevant application stack frame. Give the coder one representative report and the affected test classes. On this replay, 88 errors shared `RejectedExecutionException`. Compare their stack frames before grouping them.

Run the affected classes in their original module and test phase after each repair. Confirm that Maven actually executed those tests. Use the original goal when a focused invocation cannot reproduce the failure. Once the known failures clear, run all configured goals across the reactor. This also checks previously skipped modules and detects regressions outside the affected classes.

Replay the same commit. Compare full-suite invocation count, focused test time, total healing time, and post-gate outcome. Seven minutes per avoided invocation is a gross saving. Focused test time and any additional diagnosis reduce it.
