-- name: GetRepo :one
SELECT * FROM repos
WHERE id = $1;

-- name: GetRepoByURL :one
SELECT * FROM repos
WHERE url = $1;

-- name: ListRepos :many
SELECT * FROM repos
ORDER BY created_at DESC;

-- name: CreateRepo :one
INSERT INTO repos (url, branch, commit_sha)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteRepo :exec
DELETE FROM repos
WHERE id = $1;
