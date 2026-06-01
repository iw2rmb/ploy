-- name: GetWave :one
SELECT id, mig_id, spec_id, created_by, status, created_at, started_at, finished_at, stats
FROM waves
WHERE id = $1;

-- name: ListWaves :many
SELECT id, mig_id, spec_id, created_by, status, created_at, started_at, finished_at, stats
FROM waves
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: ListWavesByMig :many
SELECT id, mig_id, spec_id, created_by, status, created_at, started_at, finished_at, stats
FROM waves
WHERE mig_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CreateWave :one
INSERT INTO waves (id, mig_id, spec_id, created_by, status, started_at)
VALUES ($1, $2, $3, $4, 'Started', now())
RETURNING id, mig_id, spec_id, created_by, status, created_at, started_at, finished_at, stats;

-- name: UpdateWaveStatus :exec
UPDATE waves
SET status = $2,
    finished_at = CASE
      WHEN $2 IN ('Cancelled'::wave_status, 'Finished'::wave_status) THEN COALESCE(finished_at, now())
      ELSE NULL
    END
WHERE id = $1;

-- name: UpdateWaveCompletion :exec
UPDATE waves
SET status = $2, finished_at = now(), stats = $3
WHERE id = $1;

-- name: DeleteWave :exec
DELETE FROM waves
WHERE id = $1;
