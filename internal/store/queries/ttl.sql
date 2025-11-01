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
