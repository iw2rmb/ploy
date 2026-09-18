# Stack-Agnostic Job Storage and Gradle Cache Integration

Status: desired state

## Summary

Ploy will not know where Gradle, Maven, npm, or another tool stores its cache.
Ploy will create the same isolated storage for every job. Ploy will mount that
storage at fixed generic locations.

Deploy images and entrypoint scripts will map each tool to the generic storage.
For Gradle, deploy will provide a private writable Gradle User Home, a shared
read-only dependency cache, and the existing per-node HTTP build cache.

This boundary removes language and build-tool cache rules from Ploy. It also
keeps package reuse between jobs without giving two containers the same
writable Gradle User Home.

## Scope

This design covers:

- one universal host directory layout for every job;
- one universal container mount layout for gate and migration jobs;
- artifact selection from the universal job layout;
- generic node-cache and node-configuration mounts;
- Gradle cache configuration in deploy images and scripts;
- Gradle dependency-cache publication and cleanup;
- the per-node HTTP Gradle Build Cache;
- migration of the current Maven mount to the generic job cache;
- run-local classpath materialization;
- removal of Java-specific cache code from Ploy.

This design does not cover:

- removal of stack detection used to select a gate or migration image;
- project-local `.gradle` and `build` directories;
- cache sharing between nodes;
- artifact-repository retention;
- replacement of the Gradle Build Cache server;
- a new cross-job Maven, npm, or other tool cache layer;
- changes to project-owned Gradle task inputs and outputs.

## Current Constraint

Ploy has two separate cache-mount implementations.
`buildJavaToolCacheMountsFromStackEnv` adds Gradle and Maven mounts to ordinary
container jobs. `buildToolCacheMounts` adds Gradle and Maven mounts to gate
jobs.

Both implementations convert language, tool, and release values into host
paths. Both implementations know that Gradle uses `/root/.gradle` and Maven
uses `/root/.m2`.

The complete `/root/.gradle` directory is writable and shared by concurrent
containers. A job can therefore change locks, metadata, compiled scripts, and
transforms while another job reads them.

The mount also hides Gradle policy that the image stores in `/root/.gradle`.
The effective container configuration is then different from the image
configuration.

Gate and migration execution build their mounts through different call paths.
The two paths can therefore apply different isolation or cleanup rules to the
same tool.

Ploy also recognizes Gradle when it creates and parses
`/tmp/gradle-build-cache-hits`. This makes cache reporting another
Gradle-specific runtime contract in Ploy.

The current job-specific root is `artifacts/<job-id>`. Temporary and staging
directories have separate lifecycles. There is no single job-directory
authority. Adding writable cache or home directories below `artifacts` would
cause the repository artifact bundle to upload large caches and job
credentials.

## Architecture Decision

Ploy owns storage isolation. Deploy owns tool policy.

Ploy creates directories, validates paths, mounts directories, selects durable
artifacts, and removes job-owned runtime state. These actions do not depend on
language, stack, tool, or release.

Deploy decides how Gradle uses the generic directories. Deploy also owns
Gradle init scripts, dependency-cache generations, build-cache connection
files, and Gradle cleanup rules.

The control plane does not provide Gradle cache variables. Node deployment
provides generic host roots. Job images translate the generic container
contract into Gradle variables.

### Rejected boundaries

A shared writable generic cache does not solve the problem. Tool-neutral path
names do not prevent two containers from changing the same tool state.

A Gradle cache descriptor in Ploy also keeps the wrong owner. Ploy would still
need Gradle targets, environment names, modes, and lifecycle rules.

An image-only solution cannot expose node host storage. The node agent must
provide safe generic mounts because the image cannot create its own Docker
bind mounts.

The HTTP Gradle Build Cache cannot replace the dependency cache. It stores task
outputs, not external dependency artifacts and metadata.

## Target Contract

### Universal job directory

The node creates this directory for every job:

```text
$PLOYD_CACHE_HOME/runs/<run-id>/jobs/<job-id>/
├── cache/
├── home/
├── in/
├── out/
├── staging/
├── tmp/
├── stdout.log
├── stderr.log
├── diff.patch
└── container.inspect.json
```

`cache`, `home`, `staging`, and `tmp` are runtime state. Ploy removes these
directories after the container stops and required outputs are collected.

`in`, `out`, logs, the diff, and container inspection data are durable job
artifacts. Ploy retains these paths until run retention removes the run.

The run keeps its sticky workspace outside the job directory. The run also
keeps two shared directories:

```text
$PLOYD_CACHE_HOME/runs/<run-id>/share
$PLOYD_CACHE_HOME/runs/<run-id>/runtime-share
```

`share` contains small durable handoff files. `runtime-share` contains
non-durable files that later jobs in the same run require.

### Universal container projection

One typed job-storage value supplies all host paths to container construction.
Gate jobs and migration jobs use the same mount builder.

The mount builder creates this projection:

| Host source | Container target | Mode |
|---|---|---|
| job `in` | `/in` | read-write |
| job `out` | `/out` | read-write |
| job `tmp` | `/tmp` | read-write |
| job `cache` | `/ploy/cache/job` | read-write |
| job `home` | resolved `HOME` | read-write |
| run `share` | `/share` | read-write |
| run `runtime-share` | `/run-share` | read-write |
| node cache root | `/ploy/cache/node` | read-only |
| common node config | `/ploy/config/common` | read-only |
| job-type node config | `/ploy/config/job` | read-only |

The workspace mount remains controlled by the existing input contract. The
generic mount builder rejects duplicate targets and undeclared overlaps. It
permits a nested target only when an existing typed Ploy mount contract owns
that target, such as a read-only Hydra home input below the resolved `HOME`.

Ploy resolves `HOME` from the job manifest. The default remains `/root`. Ploy
mounts the complete job `home` directory at that path.

Ploy copies writable Hydra `home` inputs into the job `home` directory. Ploy
keeps read-only Hydra `home` inputs as nested read-only mounts. This preserves
the existing read-only contract.

Ploy sets these reserved variables for every job:

```text
PLOY_JOB_CACHE_DIR=/ploy/cache/job
PLOY_NODE_CACHE_DIR=/ploy/cache/node
PLOY_JOB_HOME_DIR=<resolved-HOME>
PLOY_RUN_SHARE_DIR=/share
PLOY_RUN_RUNTIME_SHARE_DIR=/run-share
PLOY_NODE_CONFIG_DIR=/ploy/config/common
PLOY_JOB_CONFIG_DIR=/ploy/config/job
PLOY_JOB_TYPE=<pre_gate|mig|post_gate>
```

A server environment, run environment, or job manifest cannot override these
variables.

### Generic node roots

Node deployment sets two host roots:

```text
PLOY_NODE_CACHE_ROOT=/var/cache/ploy/node-cache
PLOY_NODE_JOB_CONFIG_ROOT=/etc/ploy/job-config
```

`PLOY_NODE_CACHE_ROOT` contains reusable non-secret data. Ploy mounts the
complete root read-only into every job. Ploy does not inspect its children.

`PLOY_NODE_JOB_CONFIG_ROOT` contains deploy-owned configuration. Ploy mounts
`common` and the current job-type directory into the job. Ploy does not inspect
the files in these directories.

The deploy layout can contain tool-specific children:

```text
/var/cache/ploy/node-cache/
└── gradle/
    └── dependencies/
        ├── current -> generations/<generation-id>
        └── generations/

/etc/ploy/job-config/
├── common/
│   └── gradle/
│       └── build-cache.properties
└── pre_gate/
    └── gradle/
        └── build-cache-credentials.properties
```

These child names are a deploy contract. They are not Ploy constants.

### Artifact boundary

Ploy builds repository artifact bundles from an explicit allowlist. Ploy does
not archive the complete job directory.

The bundle includes:

- each job's `in` and `out` directories;
- each job's logs, diff, and container inspection data;
- the run `share` directory.

The bundle excludes:

- each job's `cache`, `home`, `staging`, and `tmp` directories;
- the run `runtime-share` directory;
- the node cache and node configuration roots.

The archive keeps the existing logical paths under
`artifacts/<job-id>/...` and `artifacts/shared/...`. The API and
`ploy mig fetch` format do not change when the host layout changes.

### Image home policy

A complete job-home mount hides files stored in the image home directory.
Official images must therefore store immutable defaults outside `HOME`.

An image entrypoint installs required writable defaults into the empty job
home. This applies to Codex configuration and any other image policy currently
stored below `/root`.

The installation is idempotent. A Hydra home input can replace an installed
default according to the existing input contract.

### Gradle writable state

Every Gradle-capable image sets:

```text
GRADLE_USER_HOME=$PLOY_JOB_CACHE_DIR/gradle/user-home
```

The image entrypoint creates this directory. The entrypoint copies deploy-owned
Gradle policy from an immutable image directory such as
`/usr/local/lib/ploy/gradle-user-home`.

The job is the only writer to this Gradle User Home. Gradle can safely write
locks, compiled scripts, transforms, wrapper distributions, dependency misses,
and local build-cache entries.

Ploy does not know `GRADLE_USER_HOME`. Ploy deletes the complete generic job
`cache` directory after the job.

Gradle 8 cleanup is disabled in the ephemeral Gradle User Home. Whole-directory
job cleanup makes entry-level retention unnecessary there. Older Gradle
versions use the same whole-directory job lifecycle.

### Gradle read-only dependency cache

Deploy publishes a dependency-cache generation below:

```text
$PLOY_NODE_CACHE_ROOT/gradle/dependencies/generations/<generation-id>/
└── modules-2/
```

The Gradle image entrypoint sets:

```text
GRADLE_RO_DEP_CACHE=$PLOY_NODE_CACHE_DIR/gradle/dependencies/current
```

The job sees the node cache as read-only. A dependency hit reads the published
generation. A dependency miss writes only to the job-owned Gradle User Home.

A deploy seeder writes a new empty staging Gradle User Home. The seeder uses
the exact Gradle wrappers for all configured seed projects. The seeder copies
`caches/modules-2` without lock files or `gc.properties` into a new generation.

The seeder verifies the new generation with offline resolution. It publishes
the generation only after every configured check succeeds.

The deploy maintenance lock waits until no Ploy job container is running. The
seeder then switches `current` atomically. The same idle interval permits
removal of retired generations.

Deploy never changes or removes a generation while a job is running. Ploy does
not select, mutate, or clean a Gradle generation.

### Per-node Gradle task-output cache

The existing `gradle-build-cache` service remains node-local. Deploy owns the
endpoint, server configuration, credentials, volume, and retention.

The common job configuration supplies the endpoint. The `pre_gate` job
configuration supplies the writer credential. No other job-type configuration
contains that credential.

The Gradle init script reads these deploy-owned files. All Gradle jobs enable
remote reads. The script enables remote writes only when `PLOY_JOB_TYPE` is
`pre_gate` and the writer credential file exists.

The HTTP service permits anonymous reads and rejects anonymous writes. The
service owns entry retention and size enforcement inside its Docker volume.

Project code in an authorized `pre_gate` can read its writer credential. The
job-type boundary prevents accidental credential delivery to other jobs. It
does not protect against deliberate disclosure by authorized project code.

Ploy does not inject `PLOY_GRADLE_BUILD_CACHE_URL`,
`PLOY_GRADLE_BUILD_CACHE_PUSH`, or Gradle credentials. Ploy does not traverse
the build-cache volume.

### Gradle cache evidence

The Gradle init script writes cache-hit evidence to:

```text
/out/gradle-build-cache-hits.txt
```

Ploy uploads the file through the normal `/out` artifact contract. Gradle also
writes the existing marker to the job log.

Ploy does not create a Gradle-only mount. Ploy does not parse Gradle task
names into gate metadata.

### Run-local classpath handoff

The Gradle gate script copies external classpath entries below:

```text
/run-share/java-classpath/<content-sha256>/
```

For Gradle cache entries, the script retains the
`modules-2/files-2.1/<group>/<artifact>/<version>/<source-hash>/<file-name>`
suffix. This suffix lets downstream tools recover dependency coordinates when
JAR metadata is absent. Other external entries use `<file-name>`.

The Gradle gate script writes the materialized paths to
`/share/java.classpath`. Workspace paths remain below `/workspace`.

The downstream image reads `/share/java.classpath` and `/run-share`. Ploy only
provides the two generic run mounts.

The artifact bundle includes the small classpath manifest. The artifact bundle
excludes the materialized dependency content.

### Cleanup ownership

Ploy removes `cache`, `home`, `staging`, and `tmp` as complete directories
after each job. This removal is safe while another job runs because no other
job uses these directories.

After each job, Ploy starts an abandoned-runtime sweep when the node has no
active job. Ploy runs the same sweep during node startup reconciliation. The
sweep removes only runtime directories for terminal jobs.

Ploy removes `runtime-share` when the run reaches its terminal retention
point. Ploy keeps durable job artifacts according to the existing run
retention contract.

Deploy replaces a dependency generation as a complete directory. A new
generation starts from an empty staging cache and contains only dependencies
required by the configured seed inputs. Retiring the old generation removes
packages that are no longer in the seed set.

The HTTP Gradle Build Cache service removes old task outputs according to its
own retention and size settings.

The host cleanup script can remove an old complete run after the node is idle.
It does not delete individual files below a Gradle User Home, a published
dependency generation, or the HTTP build-cache volume.

## Enforcement

Ploy will define one `JobDirectories` value in `internal/nodeagent`. The value
will be the only authority for job `cache`, `home`, `in`, `out`, `staging`,
`tmp`, logs, diff, and inspection paths.

`internal/nodeagent` will convert `JobDirectories` into one typed
`internal/workflow/step.JobMounts` value. Only `JobMounts` will cross into
ordinary container execution and gate execution. Loose path arguments and the
gate share-directory context value will be removed. This dependency direction
keeps `internal/workflow/step` independent of `internal/nodeagent`.

The common mount builder will reject an empty source, a relative target, a
target escape, a duplicate target, an undeclared overlap, or an unexpected
writable node mount. A typed nested-mount contract is the only permitted
overlap.

The node-cache mount will always be read-only. No job manifest can change its
source, target, or mode.

Artifact bundle tests will place files in every job directory. The archive
must include only the durable allowlist and must keep the existing archive
names.

Concurrent-job tests will run two same-image jobs. The tests will require
different `cache`, `home`, `tmp`, and `staging` host sources.

Gate and migration tests will require the same generic mount targets. No test
will select a mount from language, tool, or release.

Repository checks will reject cache-path constants for `/root/.gradle` and
`/root/.m2` in Ploy runtime code.

Deploy image tests will mount empty job `home` and `cache` directories. The
tests will verify that Codex defaults and Gradle policy are installed at
runtime.

Deploy Gradle tests will require a writable private Gradle User Home and a
read-only node dependency cache. A dependency miss must not change the node
cache.

Deploy service tests will require the maintenance lock to observe Ploy job
labels before a generation switch or retirement.

## Implementation Slices

Each slice has one subject and one observable result. The sequence keeps every
deployed revision operational.

### 1. Add the universal job directory

Owning worktree: `/Users/v.v.kovalev/@iw2rmb/ploy-gradle`.

Replace `jobArtifactPaths` with `JobDirectories`. Create `jobs/<job-id>` with
the complete target layout. Keep the existing artifact upload format by
building explicit archive entries from durable paths. Remove runtime
directories after container completion. Run an idle abandoned-runtime sweep
after job completion and during startup reconciliation. Keep durable paths
until run cleanup.

The slice is complete when gate and migration jobs use the same physical job
layout and `ploy mig fetch` returns the unchanged logical artifact layout.

### 2. Add generic mounts and node roots

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy-gradle` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Add the typed common mount builder. Convert `JobDirectories` to
`internal/workflow/step.JobMounts` in `internal/nodeagent`. Pass only
`JobMounts` to gate and migration execution. Add `PLOY_NODE_CACHE_ROOT` and
`PLOY_NODE_JOB_CONFIG_ROOT` to node deployment. Create empty deploy-owned
roots. Add the generic job-cache, node-cache, and node-configuration mounts.
Keep the complete job-home and runtime-share mounts disabled in this slice.

The slice is complete when a non-Java gate job and a migration job receive the
same generic cache and configuration variables and mount targets.

### 3. Make the job home complete

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy-gradle` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Mount the complete job `home` at the resolved `HOME`. Seed Hydra home inputs
through the job-home contract. Move official image defaults out of `/root`.
Install Codex and other writable defaults from each image entrypoint.

The slice is complete when official images start with an empty mounted home,
load their defaults, and preserve Hydra read-only home inputs.

### 4. Move Gradle writable state to the generic job cache

Owning worktree: `/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Move Gradle policy out of `/root/.gradle`. Change every Gradle-capable image
entrypoint to set `GRADLE_USER_HOME` below `PLOY_JOB_CACHE_DIR` and install the
policy there. Disable Gradle entry cleanup in this ephemeral directory.

The slice is complete when two concurrent Gradle jobs have different writable
Gradle User Homes and both load the same image policy.

### 5. Publish the Gradle dependency cache

Owning worktree: `/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Add the dependency seeder, offline verification, immutable generation
publication, and idle-only retirement. Change the Gradle entrypoint to set
`GRADLE_RO_DEP_CACHE` from `PLOY_NODE_CACHE_DIR`.

The slice is complete when a dependency hit reads the node cache, a dependency
miss changes only the job cache, and a running job cannot observe a generation
change.

### 6. Connect the Gradle task-output cache

Owning worktree: `/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Add common build-cache configuration and the `pre_gate` writer credential.
Change the Gradle init script to read deploy-owned configuration files. Write
cache-hit evidence below `/out`.

The slice is complete when a clean `pre_gate` stores an output, a separate job
reuses the output, and an anonymous HTTP write fails.

### 7. Make classpath handoff independent of cache paths

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy-gradle` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Mount generic `runtime-share` in every job. Change the Gradle classpath script
to materialize external entries below `/run-share`. Keep only the path manifest
below `/share`. Exclude `runtime-share` through the artifact allowlist.

The slice is complete when a downstream job reads the classpath after the
producer's complete job cache has been removed.

### 8. Move Maven writable state to the generic job cache

Owning worktree: `/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Move Maven settings out of `/root/.m2`. Change Maven-capable image entrypoints
to select a job-private local repository below `PLOY_JOB_CACHE_DIR`. Continue
to use the configured artifact repository for cross-job package reuse.

The slice is complete when two concurrent Maven jobs use different writable
local repositories and both load the deploy-owned Maven settings.

### 9. Remove tool-cache policy from Ploy

Owning worktree: `/Users/v.v.kovalev/@iw2rmb/ploy-gradle`.

Delete `buildJavaToolCacheMountsFromStackEnv`, `buildToolCacheMounts`,
`toolCacheTarget`, Gradle and Maven home constants, and the Gradle cache-hit
mount and parser. Remove `PLOY_BUILDGATE_CACHE_ROOT` from Ploy configuration
and diagnostics. Replace its storage aggregate with `PLOY_NODE_CACHE_ROOT`.
Update the current environment documentation, node-maintenance documentation,
and heartbeat and diagnostics API descriptions in the same slice. Keep stack
values only where they select images or build commands.

The slice is complete when Ploy runtime code contains no Gradle or Maven cache
path and all cache tests use only the generic job-storage contract.

### 10. Retire legacy host cache trees

Owning worktree: `/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Drain each node. Quarantine the old `java/gradle/<release>` and
`java/maven/<release>` writable homes. Remove Gradle and Maven entry-level
deletion from `ploy-node-cleanup`. Keep old-run cleanup and immutable Gradle
generation maintenance as separate whole-directory operations.

The slice is complete when a cleanup run does not traverse a Gradle User Home
or modify a published dependency generation.

## Completion

The design is complete when all of these statements are true:

- Ploy creates the same job directories for every stack.
- Gate and migration containers use one generic mount builder.
- Two concurrent jobs never share writable `cache`, `home`, `tmp`, or
  `staging` sources.
- The node cache is always mounted read-only.
- Artifact bundles exclude all runtime and node cache content.
- Existing artifact archive paths remain compatible.
- Ploy runtime code has no Gradle or Maven cache path.
- Every official image works with an empty mounted job home.
- Every Gradle image uses a job-private writable Gradle User Home.
- Every Gradle image reads the same immutable image policy.
- Every Maven image uses a job-private writable local repository.
- A Gradle dependency hit reuses the node read-only cache.
- A Gradle dependency miss writes only to the job cache.
- A running job cannot observe a dependency-generation change.
- Only `pre_gate` receives the Gradle build-cache writer configuration.
- An anonymous HTTP build-cache write is rejected.
- A separate job can reuse a task output from the node HTTP cache.
- Gradle cache-hit evidence is a normal `/out` artifact and log entry.
- A downstream job can consume a materialized classpath after producer cache
  cleanup.
- Host cleanup does not delete entries inside a Gradle-owned directory.
- A beta canary completes two concurrent Gradle runs without cache
  deserialization errors.

## References

- [Gradle dependency caching](https://docs.gradle.org/current/userguide/dependency_caching.html)
- [Gradle-managed directories and cleanup](https://docs.gradle.org/current/userguide/directory_layout.html)
- [Gradle build cache](https://docs.gradle.org/current/userguide/build_cache.html)
- [Gradle Build Cache Node user manual](https://docs.gradle.com/build-cache-node)
- [`internal/nodeagent/execution_paths.go`](../internal/nodeagent/execution_paths.go)
- [`internal/nodeagent/execution_artifacts.go`](../internal/nodeagent/execution_artifacts.go)
- [`internal/workflow/step/container_spec.go`](../internal/workflow/step/container_spec.go)
- [`internal/workflow/step/gate_docker_mounts.go`](../internal/workflow/step/gate_docker_mounts.go)
- [`internal/workflow/step/java_tool_cache_mounts.go`](../internal/workflow/step/java_tool_cache_mounts.go)
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/images/docker-compose.yml`
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/images/gates/gradle/cache.init.gradle`
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/services/bin/ploy-node-cleanup`
