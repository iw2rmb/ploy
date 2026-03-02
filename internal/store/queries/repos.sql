-- name: GetRepo :one
SELECT id, url, created_at
FROM repos
WHERE id = $1;
