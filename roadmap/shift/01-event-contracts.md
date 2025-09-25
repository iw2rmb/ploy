# Event Contract Rollout
- [x] Done

## Why / What For
Define the JetStream subjects, message schemas, and retention policies that let Ploy stay stateless while Grid retains control-surface ownership.

## Required Changes
- Publish subject map (`grid.webhook.<tenant>`, `ploy.workflow.<ticket>.checkpoints`, etc.) with schema docs.
- Build protobuf/JSON schema packages for workflow checkpoints, cache key announcements, and artifact manifests.
- Implement migration notes for tearing out Nomad topic consumers and wiring CLI stubs into JetStream.

## Definition of Done
- Schema definitions live in repo with versioning strategy and example payloads.
- CLI skeleton can pull a ticket and write a checkpoint message to JetStream in a local stub environment.
- Legacy Nomad/Consul references removed from messaging packages.

## Tests
- Unit tests for schema validation helpers and message round-tripping.
- Contract tests using NATS JetStream stub to ensure durable consumer config matches expectations.
- Documentation lint (markdown link checks) covering published subject map.
