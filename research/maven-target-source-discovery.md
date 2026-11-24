# Distinguish Maven target output from Java packages named target

Maven normally writes build output to `<module>/target/`. This repository also uses `ru.tinkoff.target` as a Java package. Native OpenRewrite discovery prunes directories named `target` before the runner assigns Java files to their source groups.

## Evidence from the same commit

Both runs used `ms/cb-platform-gateway:f197631e18af9937f31058a3c73b1d1d49e22816`.

- Deploy commit `36e11ac` made the runner walk declared Java source roots. On November 23 the first recipe parsed 1,491 files and applied 122 results. Its patch changed 117 Java files and no POMs. The second parser saw Boot 4 and Jackson 3 imports with Boot 3.5.8 and Jackson 2.19.4 on its classpath. During type resolution, Java 25 raised an assertion in `DeferredAttr$DeferredType.complete`. The trace did not identify the triggering expression. [November 23 run](https://ploy.t-tech.team/v1/runs/3MVexacxa3A7MEMkE6E7GGi7Qqe/status)
- Deploy commit `6ca3ee9` restored `OmniParser.acceptedPaths(workspace)` in `RewriteCliMain.parseWorkspace`. The regression test `nativeDiscoverySkipsTargetPackageUnderDeclaredSourceRoot` explicitly expects the package to be skipped. On November 24, both recipes parsed 15 files and applied zero changes. Of 1,145 tracked Java files, 1,144 were under a package directory named `target`. The remaining file was outside the declared roots. [November 24 run](https://ploy.t-tech.team/v1/runs/3MXYUrSX6JbDi1WWCIxhfeEPsNg/status)

## Proposed experiment

For Maven Java discovery, prune the standard output subtree relative to each module. Admit Java packages named `target` inside declared source roots. Preserve other exclusions and native resource discovery. The existing manifest supplies module paths and source roots. Supporting custom build directories would require the effective Maven build directory.

| Path | Expected discovery |
|---|---|
| `core/target/generated-sources/Foo.java` | Skip as module output |
| `core/src/main/java/ru/tinkoff/target/Foo.java` | Include in the main source group |
| `core/src/test/java/ru/tinkoff/target/FooTest.java` | Include in the test source group |

Exercise all three paths in the runner regression test. Then replay both recipes in sequence on the same commit. Verify Java coverage, the first recipe's source and POM changes, the second parser's classpath, and its exit status. A discovery test cannot establish that the second parser accepts the migrated source. Refreshing dependencies from unchanged POMs would still request the previous baseline.
