-- name: CreateTransformationOutcome :one
INSERT INTO transformation_outcomes (
    transformation_id, recipe_id, success, duration_seconds,
    language, framework, pattern_signature, codebase_size,
    complexity_score, strategy, error_type, error_message,
    performance_impact, metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: GetTransformationOutcome :one
SELECT * FROM transformation_outcomes WHERE id = $1;

-- name: GetTransformationOutcomesByPattern :many
SELECT * FROM transformation_outcomes 
WHERE pattern_signature = $1 
ORDER BY created_at DESC 
LIMIT $2 OFFSET $3;

-- name: GetTransformationOutcomesByLanguage :many
SELECT * FROM transformation_outcomes 
WHERE language = $1 
  AND created_at >= $2 
ORDER BY created_at DESC;

-- name: GetSuccessfulOutcomes :many
SELECT * FROM transformation_outcomes 
WHERE success = true 
  AND language = $1 
  AND created_at >= $2 
ORDER BY duration_seconds ASC;

-- name: GetFailedOutcomes :many
SELECT * FROM transformation_outcomes 
WHERE success = false 
  AND language = $1 
  AND created_at >= $2 
ORDER BY created_at DESC;

-- name: GetOutcomeStatistics :one
SELECT 
    COUNT(*) as total_count,
    COUNT(*) FILTER (WHERE success = true) as success_count,
    AVG(duration_seconds) as avg_duration,
    AVG(complexity_score) as avg_complexity
FROM transformation_outcomes 
WHERE language = $1 
  AND created_at >= $2;