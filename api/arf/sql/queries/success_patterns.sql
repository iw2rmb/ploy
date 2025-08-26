-- name: CreateSuccessPattern :one
INSERT INTO success_patterns (
    signature, language, success_rate, occurrence_count,
    avg_duration, confidence_level, factors, conditions
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: UpdateSuccessPattern :one
UPDATE success_patterns 
SET success_rate = $2,
    occurrence_count = $3,
    avg_duration = $4,
    confidence_level = $5,
    factors = $6,
    conditions = $7,
    updated_at = NOW()
WHERE signature = $1 
RETURNING *;

-- name: GetSuccessPattern :one
SELECT * FROM success_patterns WHERE signature = $1;

-- name: GetSuccessPatternsByLanguage :many
SELECT * FROM success_patterns 
WHERE language = $1 
  AND confidence_level >= $2 
ORDER BY success_rate DESC, occurrence_count DESC;

-- name: GetTopSuccessPatterns :many
SELECT * FROM success_patterns 
WHERE success_rate >= $1 
  AND occurrence_count >= $2 
ORDER BY success_rate DESC, confidence_level DESC 
LIMIT $3;

-- name: SearchSuccessPatterns :many
SELECT * FROM success_patterns 
WHERE (language = $1 OR $1 = '') 
  AND success_rate >= $2 
  AND confidence_level >= $3 
  AND updated_at >= $4 
ORDER BY success_rate DESC;

-- name: DeleteOldSuccessPatterns :exec
DELETE FROM success_patterns 
WHERE updated_at < $1 
  AND occurrence_count < $2;