-- name: UpsertJobMetric :exec
INSERT INTO job_metrics (
  node_id,
  job_id,
  cpu_consumed_ns,
  disk_consumed_bytes,
  mem_consumed_bytes
) VALUES (
  $1, $2, $3, $4, $5
)
ON CONFLICT (job_id) DO UPDATE
SET
  node_id = EXCLUDED.node_id,
  cpu_consumed_ns = EXCLUDED.cpu_consumed_ns,
  disk_consumed_bytes = EXCLUDED.disk_consumed_bytes,
  mem_consumed_bytes = EXCLUDED.mem_consumed_bytes;
