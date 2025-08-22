-- name: CreateFailurePattern :one
INSERT INTO failure_patterns (
    signature, frequency, failure_rate, 
    common_errors, context_factors, mitigations
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: UpdateFailurePattern :one
UPDATE failure_patterns 
SET frequency = $2,
    failure_rate = $3,
    common_errors = $4,
    context_factors = $5,
    mitigations = $6,
    updated_at = NOW()
WHERE signature = $1 
RETURNING *;

-- name: GetFailurePattern :one
SELECT * FROM failure_patterns WHERE signature = $1;

-- name: GetHighRiskFailurePatterns :many
SELECT * FROM failure_patterns 
WHERE failure_rate >= $1 
  AND frequency >= $2 
ORDER BY failure_rate DESC, frequency DESC;

-- name: GetRecentFailurePatterns :many
SELECT * FROM failure_patterns 
WHERE updated_at >= $1 
ORDER BY frequency DESC 
LIMIT $2;

-- name: IncrementFailurePattern :one
UPDATE failure_patterns 
SET frequency = frequency + 1,
    updated_at = NOW()
WHERE signature = $1 
RETURNING *;