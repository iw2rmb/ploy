# Reduce full Maven test runs during healing

## Summary

After each repair, `maven_tests_audit_cycle` restarts the configured Maven goals because it retains only one failing report. Use all fresh reports from the failed invocation to select a repair, then run the affected classes. Only an unfiltered pass of every configured goal across the Maven reactor completes the audit. The reactor comprises the modules Maven builds together.

## Scope

The Java migration repository owns this change in `healing/maven-tests.yaml`. Keep the existing goal discovery, Maven wrapper selection, compilation flow, coder repair rules, and Ploy post-gate. The experiment covers Surefire and Failsafe reports in the directories the flow already searches. Gradle healing, Maven parallelism, and Ploy API changes are outside this slice.

## Observed Constraint

`maven_tests_audit` reads `maven.goals` from `/in/test-tasks.yaml`. Without configured goals, it selects `test` and adds `verify` when it finds Failsafe indicators. It writes the ordered goals to `/out/amata/maven-tests/goals.txt`.

`maven_tests_audit_cycle` invokes each goal separately from the root POM and stops at the first nonzero exit. Its `find_failed_report_since` function sorts fresh report paths and stops at the first XML file containing a failure or error. The coder receives that path and the command log. The recursive call restarts the goal list. A later invocation can overwrite the same report under the module’s `target/` directory.

In the Spring Boot 3.5 to 4.0 replay, the first invocation ran 2,025 tests and reported 4 failures and 1,303 errors. The fourth reported 8 failures and 128 errors. One repair cleared a single assertion in `OfferControllerTest`. Of the errors, 88 shared the type `RejectedExecutionException`. Their stack frames must establish which failures share an operation. Later full runs took about seven minutes each. [Replay research](../research/maven-healing-test-reruns.md)

## Target Contract

1. **Collect evidence per invocation.** Read all fresh testcase XML under the existing Surefire and Failsafe report directories. Record the module path relative to the reactor root, report family, class, testcase, failure or error, and stack trace. Preserve representative XML and pending testcase evidence before launching Maven again. When reports are unreadable or a failed goal produces no actionable testcase evidence, give the coder the original command log.
2. **Select one repair.** Group failures by the deepest reported exception type and the first frame in that cause that maps to project source. Match the declaring class and method. Keep a testcase separate when either component is ambiguous. Choose the group with the most failing testcases. Break ties by the first member in module, report path, and testcase order. Give the coder one representative report from that member, the frame when available, the affected classes by module and report family, and the command log. Preserve the existing repair rules.
3. **Run the affected classes.** Partition the group by module and report family. Invoke the original goal from the reactor root with the existing Maven executable, root POM, and flags. Select the module with `-pl`. Supply exact qualified class names using `-Dtest` for Surefire or `-Dit.test` for Failsafe. The report directory determines the selector even when Surefire fails during `verify`. Preserve the lifecycle surrounding integration tests.
4. **Verify what ran.** Inspect fresh reports from the selected module and family. Require a non-skipped testcase for every requested class. Where the original failing testcase has a stable identity, require evidence that it executed. Exit code zero plus this evidence gives a provisional pass. A focused pass alone cannot establish that an order-dependent failure cleared.
5. **Return to the original goal when focus is inconclusive.** Run it across the reactor with the original arguments when module or selector resolution fails, dependencies cannot resolve, coverage is incomplete, or the failure requires the original invocation to reproduce. Use that result before asking the coder for another repair. Keep a recurring failure on this unfiltered path once focus has proved inconclusive for it.
6. **Reconcile the evidence.** Replace outcomes only for testcases that the focused invocation demonstrably executed. Remove observed passes from pending work, retain fresh failures, and add newly observed failures. A class can contribute failures to multiple groups. Recompute groups after reconciliation and retain pending evidence for untouched classes. An unfiltered repeat of the failed goal establishes a new collection for that goal.
7. **Verify the reactor.** When pending failures clear, start at the first entry in `goals.txt` and execute the whole sequence without selectors. Stop at the first failed goal and collect its evidence. This run covers previously skipped modules and detects failures outside the selected classes. Return success only when the whole sequence passes after the last repair.

## Enforcement

`maven_tests_audit_cycle` owns command selection, coder invocation, and the final success decision. Use one helper in the Java migration’s `healing/` directory to parse XML and reconcile testcase outcomes. Use the Maven image’s JDK XML parser. The coder edits the workspace and receives the evidence selected by the flow.

Keep one record per actual Maven invocation under `/out/amata/maven-tests`. Retain the invocation kind (focused or full reactor), argument vector, elapsed time, exit code, command log, and fresh report evidence. Use invocation-specific paths so that later commands preserve the evidence for pending repairs. Derive coder input and replay measurements from these records. Read the final goal sequence from the existing `goals.txt`.

## Implementation Slice

Deliver the collector, focused execution, and full reactor verification together in the Java migration repository.

- Replace `find_failed_report_since` with invocation-scoped XML collection. Exercise grouping and evidence retention with report fixtures.
- Replace the unconditional recursive restart with the transitions in the target contract. Preserve log-based repair for an unfiltered Maven failure without usable XML.
- Package the helper through `healing/mig.yaml`. Update the Maven section of `healing/README.md` and exercise the flow through its Amata entry point.

## Completion

| Scenario | Required observation |
| --- | --- |
| Several classes report one exception type | Matching project frames form one group. Different or ambiguous frames remain separate. The largest group supplies the representative report. |
| A later invocation overwrites a report | The coder can still read the representative evidence for an untouched group. New outcomes update all affected groups. |
| Two modules contain the same class name | Only evidence from the requested module and report family satisfies coverage. Surefire under `verify` and Failsafe integration tests use their respective selectors. |
| Focus returns zero with stale, skipped, empty, or partial results | The flow repeats the unfiltered goal. A selector or dependency-resolution error also takes this path before the coder receives a repair request. |
| An unfiltered build fails without readable testcase evidence | The coder receives the original goal, arguments, exit code, and command log. |
| An earlier failure prevented a later module from running | Full verification exposes the later failure. Audit success requires a subsequent complete pass. |

Replay `ms/cb-platform-gateway` at `f197631e18af9937f31058a3c73b1d1d49e22816`. Keep migration inputs the same except for the healing change. Compare against run `3MXYUrSX6JbDi1WWCIxhfeEPsNg`, whose post-gate succeeded. Count each unfiltered test-goal invocation separately, including fallback and final verification. Report cumulative focused Maven time, total healing elapsed time, and post-gate outcome alongside that count. Identify unavailable measurements. Seven minutes per avoided invocation estimates gross savings. Use measured healing time to assess the net result. [Baseline run](https://ploy.t-tech.team/v1/runs/3MXYUrSX6JbDi1WWCIxhfeEPsNg/status)

## References

- [Java migration Maven healing flow](../../../@gitlab/ploy/java/healing/maven-tests.yaml), [Amata entry point](../../../@gitlab/ploy/java/healing/amata.yaml), and [migration input packaging](../../../@gitlab/ploy/java/healing/mig.yaml).
- Apache Maven documents class selection for [Surefire](https://maven.apache.org/surefire/maven-surefire-plugin/examples/single-test.html) and [Failsafe](https://maven.apache.org/surefire/maven-failsafe-plugin/examples/single-test.html).
