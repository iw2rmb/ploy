# Mods Report: java11to17-orw-llm

## Summary
- Repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
- Branch: workflow/java11to17-orw-llm/1758400788
- Merge Request: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/68
- Started: 2025-09-20T20:39:47Z
- Ended: 2025-09-20T20:41:43Z
- Duration: 1m56.135532688s

## Happy Path
1. [success] clone (system)
   - Message: Cloned https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git at e2e/fail-missing-symbol
2. [success] create-branch (system)
   - Message: Created workflow branch: workflow/java11to17-orw-llm/1758400788
3. [success] baseline-build
   - Message: Baseline build completed successfully (post-healing)
4. [success] java11to17-migration (orw-apply)
   - Message: Applied ORW diff
   - Recipes:
     * org.openrewrite.java.migrate.UpgradeToJava17 (org.openrewrite.recipe:rewrite-migrate-java@3.17.0)

```diff
diff --git a/pom.xml b/pom.xml
index 32d75b2..ca9f41c 100644
--- a/pom.xml
+++ b/pom.xml
@@ -14,8 +14,8 @@
     <description>Test Java 11 project for OpenRewrite migration testing</description>
 
     <properties>
-        <maven.compiler.source>11</maven.compiler.source>
-        <maven.compiler.target>11</maven.compiler.target>
+        <maven.compiler.source>17</maven.compiler.source>
+        <maven.compiler.target>17</maven.compiler.target>
         <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
         <junit.version>5.9.3</junit.version>
         <commons-lang3.version>3.12.0</commons-lang3.version>
@@ -64,10 +64,9 @@
             <plugin>
                 <groupId>org.apache.maven.plugins</groupId>
                 <artifactId>maven-compiler-plugin</artifactId>
-                <version>3.11.0</version>
+                <version>3.14.0</version>
                 <configuration>
-                    <source>11</source>
-                    <target>11</target>
+                    <release>17</release>
                 </configuration>
             </plugin>
             
```
5. [success] commit
   - Message: Committed changes
6. [success] build
   - Message: Build completed successfully
7. [success] push
   - Message: Pushed branch workflow/java11to17-orw-llm/1758400788
8. [success] mr
   - Message: MR created: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/68

## Step Tree
- [success] clone (system) — Cloned https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git at e2e/fail-missing-symbol
- [success] create-branch (system) — Created workflow branch: workflow/java11to17-orw-llm/1758400788
- [failed] baseline-build (build) — docker build failed: exit status 1
builder job: mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788
builder logs archived at build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log (http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log)
download full builder log via https://api.dev.ployman.app/v1/apps/mod-java11to17-orw-llm-1758400788/builds/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788/logs/download
error code: build_failed
{"id":"mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788","app":"mod-java11to17-orw-llm-1758400788","lines":1200,"logs":""}: build check unsuccessful (controller=https://api.dev.ployman.app/v1 app=mod-java11to17-orw-llm-1758400788 lane=D env=dev): docker build failed: exit status 1
builder job: mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788
builder logs archived at build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log (http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log)
download full builder log via https://api.dev.ployman.app/v1/apps/mod-java11to17-orw-llm-1758400788/builds/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788/logs/download
error code: build_failed
{"id":"mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788","app":"mod-java11to17-orw-llm-1758400788","lines":1200,"logs":""}
  • Addressed Error: docker build failed: exit status 1
builder job: mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788
builder logs archived at build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log (http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log)
download full builder log via https://api.dev.ployman.app/v1/apps/mod-java11to17-orw-llm-1758400788/builds/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788/logs/download
error code: build_failed
{"id":"mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788","app":"mod-java11to17-orw-llm-1758400788","lines":1200,"logs":""}: build check unsuccessful (controller=https://api.dev.ployman.app/v1 app=mod-java11to17-orw-llm-1758400788 lane=D env=dev): docker build failed: exit status 1
builder job: mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788
builder logs archived at build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log (http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788.log)
download full builder log via https://api.dev.ployman.app/v1/apps/mod-java11to17-orw-llm-1758400788/builds/mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788/logs/download
error code: build_failed
{"id":"mod-java11to17-orw-llm-1758400788-d-build-20250920-203948-1758400788","app":"mod-java11to17-orw-llm-1758400788","lines":1200,"logs":""}
- [success] baseline-build — Baseline build completed successfully (post-healing)
- [success] java11to17-migration (orw-apply) — Applied ORW diff
  • Recipes:
    - org.openrewrite.java.migrate.UpgradeToJava17 (org.openrewrite.recipe:rewrite-migrate-java@3.17.0)
  • References:
    - submitted_hcl: /tmp/mods-mod-4f5be165-174065877/orw-apply/java11to17-migration/orw_apply.submitted.hcl
    - pre_hcl: /tmp/mods-mod-4f5be165-174065877/orw-apply/java11to17-migration/orw_apply.pre.hcl
    - input.tar: /tmp/mods-mod-4f5be165-174065877/orw-apply/java11to17-migration/input.tar
    - diff.patch: /tmp/mods-mod-4f5be165-174065877/orw-apply/java11to17-migration/out/diff.patch
- [success] commit — Committed changes
- [success] build — Build completed successfully
- [success] push — Pushed branch workflow/java11to17-orw-llm/1758400788
- [success] mr — MR created: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/68
