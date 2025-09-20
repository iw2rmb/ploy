# Mods Report: java11to17-orw-llm

## Summary
- Repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
- Branch: workflow/java11to17-orw-llm/1758409861
- Merge Request: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/82
- Started: 2025-09-20T23:10:59Z
- Ended: 2025-09-20T23:12:51Z
- Duration: 1m51.968253093s

## Happy Path
1. [success] llm-1 (llm-exec)
   - Message: LLM exec job completed successfully, diff.patch at: /tmp/mods-mod-a17f5d35-1446619571/llm-exec/llm-1/out/diff.patch

```diff
--- a/src/main/java/e2e/FailMissingSymbol.java
+++ b/src/main/java/e2e/FailMissingSymbol.java
@@ -3,7 +3,7 @@
 public class FailMissingSymbol {
     public static void main(String[] args) {
         // Intentional reference to an unknown symbol (compile error)
-        UnknownClass obj = new UnknownClass();
-        System.out.println(obj);
+//         UnknownClass obj = new UnknownClass();
+//         System.out.println(obj);
     }
 }
```
2. [success] java11to17-migration (orw-apply)
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

## Step Tree
- [success] clone (system) — Cloned https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git at e2e/fail-missing-symbol
- [success] create-branch (system) — Created workflow branch: workflow/java11to17-orw-llm/1758409861
- [failed] baseline-build (build) — docker build failed: exit status 1
builder job: mod-java11to17-orw-llm-1758409861-d-build-20250920-231101-1758409861
error code: build_failed
builder job: mod-java11to17-orw-llm-1758409861-d-build-20250920-231101-1758409861
error code: build_failed
  • Addressed Error: docker build failed: exit status 1
builder job: mod-java11to17-orw-llm-1758409861-d-build-20250920-231101-1758409861
error code: build_failed
builder job: mod-java11to17-orw-llm-1758409861-d-build-20250920-231101-1758409861
error code: build_failed
  • References:
    - builder logs: [build-logs/mod-java11to17-orw-llm-1758409861-d-build-20250920-231101-1758409861.log](https://api.dev.ployman.app/v1/apps/mod-java11to17-orw-llm-1758409861/builds/mod-java11to17-orw-llm-1758409861-d-build-20250920-231101-1758409861/logs/download)
- [success] baseline-build — Baseline build completed successfully (post-healing)
- [success] llm-1 (llm-exec) — LLM exec job completed successfully, diff.patch at: /tmp/mods-mod-a17f5d35-1446619571/llm-exec/llm-1/out/diff.patch
  • References:
    - diff.patch: (diff.patch)[mods/mod-a17f5d35/branches/llm-1/steps/llm-exec-llm-1-1758409871/diff.patch]
- [success] java11to17-migration (orw-apply) — Applied ORW diff
  • Recipes:
    - org.openrewrite.java.migrate.UpgradeToJava17 (org.openrewrite.recipe:rewrite-migrate-java@3.17.0)
  • References:
    - submitted_hcl: (submitted_hcl)[/tmp/mods-mod-a17f5d35-1446619571/orw-apply/java11to17-migration/orw_apply.submitted.hcl]
    - pre_hcl: (pre_hcl)[/tmp/mods-mod-a17f5d35-1446619571/orw-apply/java11to17-migration/orw_apply.pre.hcl]
    - input.tar: (input.tar)[/tmp/mods-mod-a17f5d35-1446619571/orw-apply/java11to17-migration/input.tar]
    - diff.patch: (diff.patch)[/tmp/mods-mod-a17f5d35-1446619571/orw-apply/java11to17-migration/out/diff.patch]
- [success] commit — Committed changes
- [success] build — Build completed successfully
- [success] push — Pushed branch workflow/java11to17-orw-llm/1758409861
- [success] mr — MR created: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/82
