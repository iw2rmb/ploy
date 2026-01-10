# Contract: Resource Units & Heartbeat

This file tracks *remaining* resource/heartbeat contract work that is not yet
reflected in `docs/` at HEAD.

## Resource Units

- Heartbeat should be a strict, unit-explicit contract:
  - Integer bytes for memory/disk (`mem_{free,total}_bytes`, `disk_{free,total}_bytes`).
  - Integer millicores/millis for CPU (`cpu_{free,total}_millis`) and validate fit-range.

## Identity

- Avoid redundant/ambiguous identity fields in the heartbeat body:
  - If `{id}` is in the path, remove `node_id` from the body.

