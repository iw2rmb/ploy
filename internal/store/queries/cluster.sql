-- name: GetCluster :one
SELECT * FROM cluster LIMIT 1;

-- name: CreateCluster :one
INSERT INTO cluster (id, created_at)
VALUES ($1, $2)
RETURNING *;
