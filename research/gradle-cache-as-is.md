# Gradle Cache: Current State

## Scope

This paper describes Gradle caching on a Ploy worker node as of August 3,
2026. It covers Ploy runtime mounts, Gradle gate images, the per-node Gradle
Build Cache service, and host cleanup from
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

This paper separates verified current behavior from Gradle guidance. It does
not define the target implementation.

## Summary

Ploy currently uses one persistent, writable Gradle User Home for all jobs in
the same Java release lane. This directory improves reuse, but it also combines
configuration, downloaded dependencies, compiled scripts, transforms, wrapper
distributions, and local task outputs in one shared mutation boundary.

The deploy repository also starts one HTTP Gradle Build Cache service on each
node. The service is reachable from job containers, but current jobs do not
receive its URL and the shared Gradle User Home hides the image configuration
that would enable it.

Two cleanup mechanisms act on the shared Gradle User Home. Gradle applies its
own retention rules. The host cleanup script also deletes selected Gradle
files and directories. The host script does not understand Gradle's internal
consistency rules.

## Current cache layers

### 1. Project workspace

Each run has a project workspace. Gradle writes project-local state to
`.gradle/` and task outputs to directories such as `build/` inside that
workspace.

Jobs in one run can observe files left by earlier jobs in the same workspace.
Ploy removes the complete run directory when node run retention expires.

### 2. Shared writable Gradle User Home

Ploy maps a node directory such as
`/var/cache/ploy/build/java/gradle/17` to `/root/.gradle` in a Gradle job. The
mount is writable. The release value comes from the detected Java stack.

The same directory is used by gate jobs and Java Gradle migration jobs. Two
jobs with the same release value can therefore read and write the directory at
the same time.

The mounted directory contains several different types of state:

- downloaded dependency artifacts and metadata under `caches/modules-2`;
- compiled build and init scripts under version-specific `caches` directories;
- artifact transforms and other created resources;
- local task-output cache entries;
- wrapper distributions under `wrapper/dists`;
- locks, access records, and cache cleanup state;
- Ploy cleanup init scripts under `init.d`.

The complete mount is created by
[`buildToolCacheMounts`](../internal/workflow/step/gate_docker_mounts.go). The
same mount policy is used for non-gate Java jobs by
[`buildJavaToolCacheMountsFromStackEnv`](../internal/workflow/step/java_tool_cache_mounts.go).

### 3. Image Gradle configuration

Each Gradle gate image stores `gradle.properties` and `cache.init.gradle` in
`/root/.gradle`. The properties enable task-output caching. The init script
configures the HTTP Gradle Build Cache and records cache hits.

The host bind mount replaces the image's complete `/root/.gradle` directory at
runtime. The mounted directory therefore hides both image files.

The host cleanup service adds `cache-settings.init.gradle` to the mounted
directory. This init script remains visible because the host writes it after
the mount source is created.

### 4. Per-node HTTP Gradle Build Cache

The deploy compose file starts one `gradle/build-cache-node:21.2` container on
each worker node. Its data is stored in a node-local Docker volume.

The cache container and job containers use the `ploy_default` Docker network.
The service name `gradle-build-cache` resolves to the cache container on the
same node.

The service permits anonymous reads and writes. Gradle warns that anonymous
write access can permit malicious cache entries. The current service
configuration has a target size of `10000` and a maximum artifact size of
`100`.

The control plane currently has no `PLOY_GRADLE_BUILD_CACHE_URL` or
`PLOY_GRADLE_BUILD_CACHE_PUSH` job configuration. The live beta node also has
no node-local values for these variables. Gradle jobs therefore do not receive
the HTTP endpoint.

The running service does not provide reuse by itself. A Gradle process must
enable the build cache and configure the HTTP endpoint before it reads or
writes task outputs.

## Current cleanup layers

### 1. Gradle User Home cleanup

The host cleanup service installs a Gradle 8 init script into every configured
Gradle release lane. The script sets retention for downloaded resources,
created resources, wrapper distributions, local build-cache entries, and
daemon logs.

Gradle runs User Home cleanup as part of its own lifecycle. With no daemon,
Gradle performs due cleanup in the foreground after a build session. The
default cleanup check occurs at most once in 24 hours unless the init script
selects another frequency.

Gradle versions before 8 do not use the configurable retention script. Those
versions use their built-in cleanup behavior.

### 2. Host Gradle cleanup

`ploy-node-cleanup.timer` starts the host cleanup service once per hour. The
service waits until no container with the Ploy job label is running.

The current script deletes old files below `caches/modules-2/files-2.1` and
`caches/modules-2/metadata-2.*`. The script also deletes old wrapper
distribution directories.

The current script uses file access time and `PLOY_NODE_CLEANUP_AGE`. The live
beta value is `8h`. The repository default is `24h`.

The July 16 version of the script deleted old subdirectories throughout each
Gradle release lane. That version could delete part of a compiled script cache
entry while leaving the parent entry in place.

### 3. HTTP build-cache cleanup

The Gradle Build Cache service manages its own Docker volume. The service
applies its configured target size and artifact-size limit.

The host cleanup script does not delete files inside this Docker volume. Docker
volume pruning is disabled by default.

## Confirmed failure

Run `3HGBlkcNOCeKZNDhqQwYwkT1Ixc` failed in post-gate job
`3HGBlsQrW9NXCa2sJNda0L2vvfr` on beta. Gradle failed before project task
execution.

Gradle opened this compiled init-script cache entry:

```text
/root/.gradle/caches/7.6.4/scripts/976t1y2x5xlszdy7eh4ib4ej8
```

The directory contained its lock file and `cache.properties`. The required
`metadata/metadata.bin` file was absent.

The node entry modification time was July 16. This date matches the period when
the older host cleanup algorithm deleted arbitrary old subdirectories below a
Gradle release lane. The current algorithm no longer deletes compiled script
cache directories, but it also does not repair an incomplete entry left by the
older algorithm.

## Current and possible issues

### Shared writes across containers

Gradle protects a normal dependency cache with file locks. Gradle states that
concurrent use is supported only when the Gradle processes can communicate.
Separate job containers usually do not meet this condition.

Two concurrent jobs can modify the same binary metadata, script cache,
transform cache, wrapper state, or local build cache. A failure in one job can
leave partial state that causes a later unrelated project to fail.

### Partial host deletion

The host script deletes files inside Gradle-owned directory structures. An
idle node prevents deletion during an active job, but it does not make partial
deletion consistent with Gradle metadata.

The script can remove one file that another retained Gradle file still
references. The next build can then find a cache entry that looks present but
cannot be read.

### Access-time retention

Filesystem access time is not a complete record of Gradle cache use. The live
filesystem uses `relatime`, which reduces access-time updates.

A short host retention value can classify a valid file as old even when the
logical Gradle entry remains useful. Gradle's own cleanup uses cache-specific
knowledge instead of a generic file-age rule.

### Two cleanup authorities

Gradle and the host shell both decide when Gradle-managed data is unused. The
two mechanisms use different records and different retention rules.

The host mechanism can invalidate an entry before Gradle decides that the
complete entry is unused. Gradle cannot protect its state from an external
process that deletes files while Gradle is not running.

### Hidden image policy

The shared User Home mount hides the image's Gradle properties and remote-cache
init script. Image tests can pass while runtime jobs use different effective
configuration.

This difference currently leaves the per-node HTTP service disconnected. It
also makes image ownership of Gradle policy unreliable.

### Unrestricted remote writes

The HTTP cache service accepts anonymous writes from its Docker network. Any
connected job can publish task outputs when Gradle push is enabled.

Gradle task-output reuse is correct only when cacheable tasks declare complete
inputs and outputs. A project with an incorrect task contract can publish an
incorrect result for reuse by another build.

## Gradle recommendations

### Use a read-only cache for shared dependencies

Gradle recommends a shared read-only dependency cache for ephemeral container
builds. A dedicated build creates the cache. Other containers mount the copied
`modules-2` directory read-only and set `GRADLE_RO_DEP_CACHE`.

Each container still needs a writable Gradle User Home. A missing dependency
is downloaded into that private writable cache.

Gradle requires the shared dependency cache to remain read-only while builds
use it. Gradle also says not to expose a live seeder cache as the shared
read-only cache because the live cache contains locks and can change.

### Keep mutable state private to one build

A container must not share a normal writable dependency cache with containers
that cannot participate in the same locking protocol. A private Gradle User
Home gives each build independent locks, scripts, transforms, and local state.

Different Gradle versions can use one User Home in a normal process
environment. Container isolation changes the concurrency condition and makes a
shared writable User Home unsafe.

### Let Gradle clean Gradle-managed entries

Gradle automatically cleans User Home caches and wrapper distributions. Gradle
8 and later permit retention configuration through an init script in the
applicable Gradle User Home.

An external cleanup process should delete only a complete directory whose
lifecycle it owns. It should not delete selected files inside a Gradle-managed
cache entry.

### Use the build cache for task outputs

Gradle separates dependency caching from task-output caching. The HTTP build
cache stores outputs of cacheable tasks. It does not store downloaded package
dependencies.

Gradle supports a local task-output cache and a remote HTTP task-output cache.
When both are enabled, Gradle reads the local cache first and the remote cache
second.

Gradle recommends that trusted clean continuous-integration builds populate a
shared remote cache. Other builds should read without pushing.

### Validate task cacheability before broad reuse

A cacheable task must declare all inputs and outputs. Missing declarations can
produce incorrect cache hits. Unstable inputs can produce unnecessary misses.

A build should first produce correct `UP-TO-DATE` results in repeated local
execution. Cache relocatability should also be tested before remote reuse is
treated as reliable.

## Sources

- [Gradle dependency caching](https://docs.gradle.org/current/userguide/dependency_caching.html)
- [Gradle-managed directories and cleanup](https://docs.gradle.org/current/userguide/directory_layout.html)
- [Gradle build cache](https://docs.gradle.org/current/userguide/build_cache.html)
- [Gradle Build Cache Node user manual](https://docs.gradle.com/build-cache-node)
- [`docs/envs/README.md`](../docs/envs/README.md)
- [`docs/node-maintenance.md`](../docs/node-maintenance.md)
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/images/docker-compose.yml`
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/images/gates/gradle/cache.init.gradle`
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/services/bin/ploy-node-cleanup`
