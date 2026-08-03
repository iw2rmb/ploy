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
- portable Java classpath materialization for downstream jobs;
- Gradle cache cleanup in Ploy and the deploy host services;
- recovery of abandoned job cache directories.

This design does not cover:

- Maven cache behavior;
- project-local `.gradle` and `build` directories;
- replacement of the Gradle Build Cache server product;
- a cache shared between different worker nodes;
- artifact-repository retention;
- changes to project-owned Gradle task input and output declarations;
- protection against deliberate cache credential disclosure by project code
  that Ploy authorizes as a cache producer.

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

The Gradle gate writes absolute dependency paths to `/share/java.classpath`.
Downstream ORW jobs read this file without running Gradle. The current shared
`/root/.gradle` mount keeps these paths valid across jobs.

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
$PLOY_BUILDGATE_CACHE_ROOT/java/gradle/dependencies/<generation>/
└── modules-2/
```

A job mounts the complete generation directory read-only at
`/opt/ploy/gradle-dependencies`. Ploy sets
`GRADLE_RO_DEP_CACHE=/opt/ploy/gradle-dependencies` in that job.

The active generation is immutable. A running job keeps the same resolved
generation path for its complete lifetime.

A dedicated seeder writes only to a staging Gradle User Home. An explicit
deploy maintenance action triggers the seeder with immutable project source
snapshots and their exact Gradle wrappers.

The seed configuration lists every source snapshot and Gradle version that
must populate the generation. The seeder records these inputs in a generation
manifest.

The seeder starts from a copy of the active generation when one exists. It
runs every configured seed build against the staging User Home. It uses each
seed build's wrapper to populate the cache formats required by that Gradle
version.

The seeder publishes a generation by copying the staging `caches/modules-2`
directory without lock files or `gc.properties`. It then makes the complete
generation read-only.

The seeder never writes the active generation. The seeder publishes only after
all configured seed builds and version checks succeed. An incomplete seed is
discarded.

Before publication, the seeder runs each seed input offline with an empty
writable User Home and the copied generation as `GRADLE_RO_DEP_CACHE`.
Publication fails if a configured seed cannot resolve its dependencies from
that generation.

A dependency that is absent from the read-only generation is downloaded into
the job-owned writable Gradle User Home. A cache miss cannot modify shared
state.

### Run-owned Java classpath

The Gradle gate materializes external classpath entries before Ploy deletes the
job-owned Gradle User Home. Ploy stores these entries outside the uploaded
artifact tree:

```text
$PLOYD_CACHE_HOME/runs/<run-id>/runtime-share/java-classpath/<content-sha256>/<file-name>
```

Ploy mounts the run-owned `runtime-share` directory at `/run-share` in the gate
and each downstream Java job that consumes the classpath.

The gate copies each classpath file or directory outside `/workspace` to this
layout. A directory hash covers its relative names and file content. The
content hash prevents collisions between entries with the same name.

The gate writes each materialized `/run-share/java-classpath` path to the
existing `/share/java.classpath` manifest. Workspace output paths remain below
`/workspace`.

`/share/java.classpath` cannot contain paths below `/root/.gradle` or
`/opt/ploy/gradle-dependencies`. A downstream job needs only the existing
workspace and `/share` mounts plus the run-owned `/run-share` mount.

The repository artifact upload includes `/share/java.classpath`, but it excludes
the complete `runtime-share` directory. Ploy removes `runtime-share` with the
run. The materialized classpath is a runtime handoff, not a durable artifact or
shared cache.

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

Ploy treats project code executed in an authorized `pre_gate` as trusted for
node-local cache writes. The credential boundary prevents accidental writes by
other job roles. It does not protect against deliberate disclosure by an
authorized producer.

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

Every image that invokes Gradle performs the same installation. This includes
Gradle gate images and Gradle-capable migration images.

The ORW Gradle-lane image does not install Gradle policy because it does not
run Gradle. It reads the classpath manifest from `/share` and the materialized
entries from `/run-share`.

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

The host cleanup service continues to clean non-Gradle node storage. It does
not traverse `$PLOY_BUILDGATE_CACHE_ROOT/java/gradle`.

## Enforcement

Ploy will define one typed Gradle cache layout in
`internal/workflow/step`. The layout will produce the job home, dependency
generation source, container target, and required environment from a job ID.

Container mount construction will reject a writable Gradle mount that is not
below the `jobs/<job-id>` directory. Tests will exercise two same-release jobs
and require different writable sources.

Container mount construction will set the dependency generation mount to
read-only. Tests will reject a writable dependency generation.

The Gradle cache layout will require the dependency mount source to contain a
`modules-2` child. It will set `GRADLE_RO_DEP_CACHE` to the parent container
path.

The node agent will use an allowlisted node-cache configuration type. The type
will project the HTTP endpoint into a Gradle job. It will project the writer
credentials and push permission only into `pre_gate`.

The node-cache configuration will use node-only source names that are distinct
from the job environment names. Server and per-run environment cannot override
the projected endpoint, credentials, or push permission.

The classpath collector will reject a final dependency path outside
`/workspace` and `/run-share/java-classpath`. Tests will cover a dependency
from the read-only cache and a dependency from the job-owned writable cache.

Ploy will create `runtime-share` as a sibling of `artifacts`, not a child. The
repository artifact bundler will continue to archive only `artifacts`. A bundle
test will use a materialized dependency larger than the upload limit and verify
that the dependency is absent from the archive.

Image tests will start a container with an empty directory mounted at
`/root/.gradle`. The test will verify that the effective Gradle process loads
the Ploy init scripts and configures the expected remote endpoint.

The dependency seeder will publish through a staging directory and one atomic
generation switch. The generation manifest will record seed source identities
and Gradle versions. A failed seed, offline check, copy, or permission change
will leave the previous generation active.

The deploy cleanup test will fail if the host script traverses
`$PLOY_BUILDGATE_CACHE_ROOT/java/gradle`.

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
Move image policy to an immutable image directory. Change each image that
invokes Gradle to install that policy into the job home before Gradle starts.

Add a non-uploaded run `runtime-share` directory and mount it at `/run-share`
for the gate and downstream classpath consumers. Change the gate classpath
collector to materialize external entries below
`/run-share/java-classpath` and write only portable paths before the gate exits.
After the container stops, collect required outputs and delete the job home.

Remove host-side Gradle init-script seeding after every Gradle-capable image
uses the immutable policy source. Retire legacy release-lane homes as complete
directories while the node is drained.

Deliver the Ploy and image changes as one combined rollout. The step is
complete when two concurrent same-release jobs use different writable homes
and both load the Ploy init scripts. A downstream ORW job must read the
materialized classpath after the producing job home is absent.

### Step 3: Connect the per-node task-output cache

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Add an explicit node-local build-cache endpoint and writer credential to deploy
configuration. Change the HTTP service to anonymous read access plus one named
read-write user. Add a typed node-agent projection that supplies writer access
only to `pre_gate`. Remove the Gradle build-cache variables from the documented
control-plane global configuration surface.

The step is complete when a clean producer stores a task output and a separate
job reports `FROM-CACHE`. An anonymous HTTP write must fail. A non-producer job
must receive no writer credential and must configure push as false.

### Step 4: Publish a read-only dependency generation

Owning worktrees: `/Users/v.v.kovalev/@iw2rmb/ploy` and
`/Users/v.v.kovalev/@gitlab/ploy/deploy`.

Add the deploy-owned seeder with explicit seed inputs, a Gradle-version
manifest, offline verification, staging, immutable generation publication, and
safe retirement of old generations. Add the Ploy read-only generation-root
mount and `GRADLE_RO_DEP_CACHE` projection. Resolve the active generation to a
physical path before container creation.

The step is complete when two concurrent jobs read one generation, a missing
dependency is written only to each job home, and the active generation remains
unchanged for both jobs. Every configured seed input must resolve offline with
its recorded Gradle version. A retired generation must remain until its last
container mount is gone.

### Step 5: Recover abandoned job homes when idle

Owning worktree: `/Users/v.v.kovalev/@iw2rmb/ploy`.

After every job, run an abandoned-home sweep when the node has no active jobs.
Run the same sweep during node startup reconciliation. Keep retired
dependency-generation removal in the deploy-owned seeder lifecycle.

The step is complete when an active job blocks the idle sweep and a node restart
can remove an abandoned home without modifying shared cache contents.

## Completion

The complete design is implemented when all of these checks pass:

- Two concurrent jobs never share a writable Gradle mount source.
- Every image that invokes Gradle works with an initially empty job Gradle User
  Home.
- Every shared dependency mount is read-only.
- `GRADLE_RO_DEP_CACHE` names a directory that contains `modules-2`.
- A dependency miss changes only the job-owned Gradle User Home.
- An active dependency generation cannot change.
- Every published generation passes offline resolution for its recorded seed
  inputs and Gradle versions.
- `/share/java.classpath` contains only `/workspace` or
  `/run-share/java-classpath` entries.
- Repository artifact bundles exclude `runtime-share` dependency content.
- An ORW job consumes the materialized classpath after the gate job home is
  deleted.
- A remote task-output cache hit is visible in Ploy gate metadata.
- An anonymous HTTP cache write is rejected.
- A non-producer job receives no writer credential and configures push as
  false.
- Job completion removes the complete job-owned Gradle User Home.
- Idle cleanup removes abandoned whole directories only.
- Host cleanup does not traverse the Gradle cache root.
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
