# Mods Report — java11to17-orw-llm

## Summary
- Repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
- Branch: workflow/java11to17-orw-llm/1758399315
- Merge Request: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/66
- Started: 2025-09-20T20:15:14.004925386Z
- Ended: 2025-09-20T20:17:11.827818518Z
- Duration: 1m57.822893132s

## Happy Path
1. `clone` [clone] — Cloning repository: repo=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git ref=e2e/fail-missing-symbol
   - at 2025-09-20T20:15:14.009148023Z
2. `clone-diagnostics` [clone] — repo=/tmp/mods-mod-8a520855-1233732494/repo entries=.claude,.git,README.md,pom.xml,src
   - at 2025-09-20T20:15:15.478075206Z
3. `source` [sbom] — Generated source SBOM
   - at 2025-09-20T20:15:15.483580098Z
4. `create-branch` [branch] — Creating workflow branch
   - at 2025-09-20T20:15:15.487303958Z
5. `compile-gate-start` [build] — repo=/tmp/mods-mod-8a520855-1233732494/repo sandbox=1
   - at 2025-09-20T20:15:15.497015865Z
6. `build-gate` [build] — start app=mod-java11to17-orw-llm-1758399315 lane=D env=dev wd=/tmp/mods-mod-8a520855-1233732494/repo
   - at 2025-09-20T20:15:15.518884417Z
7. `build-gate-error` [build] — {"id":"mod-java11to17-orw-llm-1758399315-d-build-20250920-201515-1758399315","app":"mod-java11to17-orw-llm-1758399315","lines":1200,"logs":""}
   - at 2025-09-20T20:15:24.870693479Z
8. `healing` [healing] — attempt 1/2
   - at 2025-09-20T20:15:24.874866445Z
9. `build-error` [healing] — docker build failed: exit status 1
builder job: mod-java11to17-orw-llm-1758399315-d-build-20250920-201515-1758399315
builder logs archived at build-logs/mod-java11to17-orw-llm-1758399315-d-build-20250920-201515-1758399315.log (http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/mod-java11to17-orw-llm-1758399315-d-build-20250920-201515-1758399315.log)
download full builder log via https://api.dev.ployman.app/v1/apps/mod-java11to17-orw-llm-1758399315/builds/mod-java11to17-orw-llm-1758399315-d-build-20250920-201515-1758399315/logs/download
error code: build_failed
{"id":"mod-java11to17-orw-llm-1758399315-d-build-20250920-201515-1758399315","app":"mod-java11to17-orw-llm-1758399315","lines":1200,"logs":""}: build check unsuccessful (controller=https://api.dev.ployman.app/v1 app=…
   - at 2025-09-20T20:15:24.878233612Z
10. `planner` [planner] — prepared inputs.json (bytes=1732)
   - at 2025-09-20T20:15:24.882277487Z
11. `sbom` [planner] — pointer missing storage_key
   - at 2025-09-20T20:15:25.542966378Z
12. `llm-exec` [fanout] — branch started: llm-1
   - at 2025-09-20T20:15:26.108921573Z
13. `orw-gen` [fanout] — uploading input.tar to http://seaweedfs-filer.storage.ploy.local:8888
   - at 2025-09-20T20:15:26.121401351Z
14. `orw-apply` [apply] — Started orw-apply
   - at 2025-09-20T20:15:26.653110997Z
15. `reducer` [reducer] — job started
   - at 2025-09-20T20:15:48.915251659Z
16. `apply` [healing] — replay starting: branch_id=llm-1 plan_id=plan-1758399325
   - at 2025-09-20T20:15:49.981726562Z
17. `post-healing-build-start` [build] — Running post-healing build gate
   - at 2025-09-20T20:15:50.074560746Z
18. `post-healing-build-succeeded` [build] — Baseline build completed successfully (post-healing)
   - at 2025-09-20T20:16:04.471243233Z
19. `build-gate-succeeded` [build] — Build version 
   - at 2025-09-20T20:16:04.478101829Z
20. `guard-build-file` [apply] — repo=/tmp/mods-mod-8a520855-1233732494/repo pom=true gradle=false kts=false
   - at 2025-09-20T20:16:04.489531285Z
21. `input-preview` [apply] — input.tar preview:
./.claude/state/context-warning-75.flag
./.claude/state/context-warning-90.flag
./.claude/state/daic-mode.json
./.git/COMMIT_EDITMSG
./.git/HEAD
./.git/ORIG_HEAD
./.git/config
./.git/config.worktree
./.git/description
./.git/hooks/applypatch-msg.sample
./.git/hooks/commit-msg.sample
./.git/hooks/fsmonitor-watchman.sample
./.git/hooks/post-update.sample
./.git/hooks/pre-applypatch.sample
./.git/hooks/pre-commit.sample
./.git/hooks/pre-merge-commit.sample
./.git/hooks/pre-push.sample
./.git/hooks/pre-rebase.sample
./.git/hooks/pre-receive.sample
./.git/hooks/prepare-commit-msg.sample
   - at 2025-09-20T20:16:04.50457966Z
22. `diff-found` [apply] — diff ready (1116 bytes)
   - at 2025-09-20T20:16:54.164238517Z
23. `diff-apply-started` [apply] — Applying diff to repository
   - at 2025-09-20T20:16:54.171106694Z
24. `diff-applied` [apply] — Diff applied
   - at 2025-09-20T20:16:54.192870944Z
25. `mr-config` [mr] — using token_env=PLOY_GITLAB_PAT
   - at 2025-09-20T20:17:08.308514248Z
26. `push` [push] — Pushing branch
   - at 2025-09-20T20:17:08.325354324Z
27. `mr` [mr] — creating MR: source=workflow/java11to17-orw-llm/1758399315 target=main
   - at 2025-09-20T20:17:10.369509791Z

## Diff Preview
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