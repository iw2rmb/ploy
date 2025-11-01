-- name: DeleteExpiredLogs :execrows
-- DeleteExpiredLogs removes log rows older than the specified timestamp.
DELETE FROM logs
WHERE created_at < $1;

-- name: DeleteExpiredEvents :execrows
-- DeleteExpiredEvents removes event rows older than the specified timestamp.
DELETE FROM events
WHERE time < $1;

-- name: DeleteExpiredDiffs :execrows
-- DeleteExpiredDiffs removes diff rows older than the specified timestamp.
DELETE FROM diffs
WHERE created_at < $1;

-- name: DeleteExpiredArtifactBundles :execrows
-- DeleteExpiredArtifactBundles removes artifact bundle rows older than the specified timestamp.
DELETE FROM artifact_bundles
WHERE created_at < $1;

-- name: ListLogPartitions :many
-- ListLogPartitions retrieves all partition names for the logs table.
SELECT format('%I.%I', n.nspname, c.relname) AS partition_name
FROM pg_catalog.pg_inherits i
JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE i.inhparent = 'ploy.logs'::regclass;

-- name: ListEventPartitions :many
-- ListEventPartitions retrieves all partition names for the events table.
SELECT format('%I.%I', n.nspname, c.relname) AS partition_name
FROM pg_catalog.pg_inherits i
JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE i.inhparent = 'ploy.events'::regclass;

-- name: ListArtifactBundlePartitions :many
-- ListArtifactBundlePartitions retrieves all partition names for the artifact_bundles table.
SELECT format('%I.%I', n.nspname, c.relname) AS partition_name
FROM pg_catalog.pg_inherits i
JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE i.inhparent = 'ploy.artifact_bundles'::regclass;

-- name: ListNodeMetricsPartitions :many
-- ListNodeMetricsPartitions retrieves all partition names for the node_metrics table.
SELECT format('%I.%I', n.nspname, c.relname) AS partition_name
FROM pg_catalog.pg_inherits i
JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE i.inhparent = 'ploy.node_metrics'::regclass;
