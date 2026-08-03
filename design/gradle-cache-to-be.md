# Gradle Cache Isolation and Reuse

Status: desired state

## Summary

The current Gradle cache path combines reusable data and job-owned mutable
state in one writable directory. Concurrent containers and host cleanup can
therefore damage state that unrelated jobs use.

Ploy will give each Gradle job a private writable Gradle User Home. Jobs will
reuse packages from an immutable read-only dependency cache. Jobs will reuse
task outputs from the per-node HTTP Gradle Build Cache.

Each cache layer will have one lifecycle owner. Ploy will own job directories.
A seeder will own dependency-cache generations. The HTTP cache service will own
remote task outputs. Gradle will own entries inside a Gradle User Home.

## Scope

This design covers:

- Gradle gate jobs and Java Gradle migration jobs;
- Gradle mounts and environment created by the Ploy node agent;
- Gradle configuration in `/Users/v.v.kovalev/@gitlab/ploy/deploy/images`;
- node-local Gradle Build Cache configuration;
- dependency-cache seeding and immutable publication;
- Gradle cache cleanup in Ploy and the deploy host services;
- recovery of abandoned job cache directories.

This design does not cover:

- Maven cache behavior;
- project-local `.gradle` and `build` directories;
- replacement of the Gradle Build Cache server product;
- a cache shared between different worker nodes;
- artifact-repository retention;
- changes to project-owned Gradle task input and output declarations.

## Observed Constraint

[`buildToolCacheMounts`](../internal/workflow/step/gate_docker_mounts.go) mounts
one writable release directory as the complete `/root/.gradle` directory. The
same mount policy is used for Java Gradle migration jobs.

The mount hides `gradle.properties` and `cache.init.gradle` that gate images
place in `/root/.gradle`. Runtime configuration therefore differs from image
configuration.

The node can run concurrent jobs. Jobs with the same Java release can write the
same Gradle User Home from separate containers.

The host cleanup script deletes selected files inside Gradle dependency caches.
An older form of the script also deleted subdirectories inside compiled script
caches.

The per-node Gradle Build Cache service is running and reachable through the
`gradle-build-cache` network name. Current job specifications do not provide
its URL to Gradle.

## Target Contract

### Job-owned writable state

Every Gradle job gets one writable Gradle User Home at:

```text
$PLOY_BUILDGATE_CACHE_ROOT/java/gradle/jobs/<job-id>
```

Only that job can mount or write the directory. The job ID is the ownership
key. Java release values do not select writable Gradle state.

The container uses the job directory as `/root/.gradle`. Gradle can write
locks, compiled scripts, transforms, wrapper distributions, local dependency
misses, and local task outputs without sharing these files with another job.

Ploy removes a completed job directory as one unit after the container stops
and required outputs are collected. Ploy never deletes selected internal files
from that directory.

### Immutable shared dependencies

Each node can expose one active dependency-cache generation at:

```text
$PLOY_BUILDGATE_CACHE_ROOT/java/gradle/dependencies/<generation>/modules-2
```

A job mounts the resolved generation read-only at
`/opt/ploy/gradle-dependencies`. Ploy sets
`GRADLE_RO_DEP_CACHE=/opt/ploy/gradle-dependencies` in that job.

The active generation is immutable. A running job keeps the same resolved
generation path for its complete lifetime.

A dedicated seeder writes only to a staging Gradle User Home. The seeder
publishes a generation by copying `modules-2` without lock files or
`gc.properties`, then making the copied generation read-only.

The seeder never writes the active generation. The seeder publishes only after
its Gradle build completes. An incomplete seed is discarded.

A dependency that is absent from the read-only generation is downloaded into
the job-owned writable Gradle User Home. A cache miss cannot modify shared
state.

### Per-node task-output cache

Each node runs one HTTP Gradle Build Cache service. The node-local endpoint is:

```text
http://gradle-build-cache:5071/cache/
```

Deploy configuration owns this endpoint. The control plane does not own a
node-local service address.

The node agent projects the endpoint into Gradle job containers. The node
agent does not copy its unrestricted process environment into jobs.

The HTTP service permits anonymous reads. The HTTP service rejects anonymous
writes.

Each node has one read-write cache user. Deploy owns its credentials. The node
agent injects the credentials only into `pre_gate`, which starts from a freshly
hydrated source workspace.

All Gradle jobs can read remote task outputs. Only `pre_gate` sets push to true
and supplies writer credentials. `post_gate` and migration jobs set push to
false and receive no writer credentials.

The HTTP service owns entry retention and size enforcement inside its Docker
volume. Ploy and the host cleanup script do not remove individual files from
that volume.

### Effective Gradle configuration

Gradle images store Ploy policy outside `/root/.gradle` in an immutable image
directory. The policy includes task-output cache configuration and required
Ploy init scripts.

An image entrypoint installs the policy into the empty job-owned Gradle User
Home before the first Gradle process starts. The installation is idempotent and
changes only Ploy-owned configuration files.

Every supported Gradle execution path performs the same installation. Gate,
migration, and ORW images cannot depend on files hidden below a runtime mount.

The remote build-cache init script reads the node-projected endpoint. For a
producer, the script also reads the node-projected writer credentials. A
project cannot select a different endpoint or push policy through
control-plane global environment configuration.

### Cleanup ownership

Gradle owns cleanup of internal entries while a Gradle User Home exists. Gradle
8 retention settings remain in an init script. Older Gradle versions use their
built-in retention behavior.

Ploy owns complete job Gradle User Home directories. After each Gradle job,
the node removes that job's directory. When no local job is running, the node
also removes abandoned completed-job directories.

The dependency seeder owns dependency-cache generations. It removes a retired
generation only when no running job has that generation mounted.

The HTTP Gradle Build Cache service owns remote task-output cleanup. Its target
size and maximum artifact size remain deployment settings.

The host cleanup service remains an emergency storage backstop. It can remove
complete abandoned job directories after it verifies that no Ploy job is
running. It cannot delete files inside Gradle-managed caches, active dependency
generations, or the HTTP cache volume.

## Enforcement

Ploy will define one typed Gradle cache layout in
`internal/workflow/step`. The layout will produce the job home, dependency
generation source, container target, and required environment from a job ID.

Container mount construction will reject a writable Gradle mount that is not
below the `jobs/<job-id>` directory. Tests will exercise two same-release jobs
and require different writable sources.

Container mount construction will set the dependency generation mount to
read-only. Tests will reject a writable dependency generation.

The node agent will use an allowlisted node-cache configuration type. The type
will project the HTTP endpoint into a Gradle job. It will project the writer
credentials and push permission only into `pre_gate`.

The node-cache configuration will use node-only source names that are distinct
from the job environment names. Server and per-run environment cannot override
the projected endpoint, credentials, or push permission.

Image tests will start a container with an empty directory mounted at
`/root/.gradle`. The test will verify that the effective Gradle process loads
the Ploy init scripts and configures the expected remote endpoint.

The dependency seeder will publish through a staging directory and one atomic
generation switch. A failed copy or failed permission change will leave the
previous generation active.

The deploy cleanup test will fail if the host script traverses an active
dependency generation or deletes below a Gradle `caches` directory.

## Implementation Slice

The migration uses five ordered delivery steps. Each step has one observable
result and leaves the node in a supported state.

### Step 1: Stop external mutation of Gradle entries

Owning worktree: `/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Remove file-level and subtree-level Gradle deletion from
`services/bin/ploy-node-cleanup`. Keep the Gradle 8 retention init script and
the idle-job safety check. Add a test that preserves files below `caches/` and
`wrapper/dists/`.

The step is complete when the host service can run on a node with no active
jobs without deleting any existing Gradle-managed entry.

### Step 2: Isolate writable Gradle User Homes

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Change Ploy to allocate `jobs/<job-id>` as the only writable Gradle User Home.
Move image policy to an immutable image directory. Change every Gradle image
entrypoint to install that policy into the job home before Gradle starts.

Deliver the Ploy and image changes as one combined rollout. The step is
complete when two concurrent same-release jobs use different writable homes
and both load the Ploy init scripts.

### Step 3: Connect the per-node task-output cache

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Add an explicit node-local build-cache endpoint and writer credential to deploy
configuration. Change the HTTP service to anonymous read access plus one named
read-write user. Add a typed node-agent projection that supplies writer access
only to `pre_gate`. Remove the Gradle build-cache variables from the documented
control-plane global configuration surface.

The step is complete when a clean producer stores a task output, a separate
job reports `FROM-CACHE`, and a non-producer cannot send an HTTP cache write.

### Step 4: Publish a read-only dependency generation

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Add the deploy-owned seeder with staging and immutable generation publication.
Add the Ploy read-only mount and `GRADLE_RO_DEP_CACHE` projection. Resolve the
active generation to a physical path before container creation.

The step is complete when two concurrent jobs read one generation, a missing
dependency is written only to each job home, and the active generation remains
unchanged for both jobs.

### Step 5: Make job completion the cleanup trigger

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Delete the current job's Gradle User Home after container shutdown and output
collection. Run an abandoned-home and retired-generation sweep when the node
has no active jobs. Keep the hourly host timer as a whole-directory recovery
backstop.

The step is complete when a finished job leaves no writable Gradle home, an
active job blocks the idle sweep, and a node restart can remove an abandoned
home without modifying shared cache contents.

## Completion

The complete design is implemented when all of these checks pass:

- Two concurrent jobs never share a writable Gradle mount source.
- Every Gradle image works with an initially empty job Gradle User Home.
- Every shared dependency mount is read-only.
- A dependency miss changes only the job-owned Gradle User Home.
- An active dependency generation cannot change.
- A remote task-output cache hit is visible in Ploy gate metadata.
- A non-producer job cannot push a remote task output.
- Job completion removes the complete job-owned Gradle User Home.
- Idle cleanup removes abandoned whole directories only.
- Host cleanup does not descend into Gradle-managed cache entries.
- The existing gate, Java tool-cache, deploy service, and Gradle cache E2E tests
  pass.
- A beta canary completes pre-gate, migration jobs, and post-gate for two
  concurrent Gradle runs without cache deserialization errors.

## References

- [Gradle dependency caching](https://docs.gradle.org/current/userguide/dependency_caching.html)
- [Gradle-managed directories and cleanup](https://docs.gradle.org/current/userguide/directory_layout.html)
- [Gradle build cache](https://docs.gradle.org/current/userguide/build_cache.html)
- [Gradle Build Cache Node user manual](https://docs.gradle.com/build-cache-node)
- [`internal/workflow/step/gate_docker_mounts.go`](../internal/workflow/step/gate_docker_mounts.go)
- [`internal/workflow/step/java_tool_cache_mounts.go`](../internal/workflow/step/java_tool_cache_mounts.go)
- [`docs/envs/README.md`](../docs/envs/README.md)
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/images/docker-compose.yml`
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/images/gates/gradle/cache.init.gradle`
- `/Users/v.v.kovalev/@gitlab/ploy/deploy/services/bin/ploy-node-cleanup`
