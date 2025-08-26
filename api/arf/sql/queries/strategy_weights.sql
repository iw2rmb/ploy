-- name: CreateOrUpdateStrategyWeight :one
INSERT INTO strategy_weights (
    strategy_name, language, pattern_type, 
    weight, performance_score, sample_size
) VALUES (
    $1, $2, $3, $4, $5, $6
) 
ON CONFLICT (strategy_name, language, pattern_type) 
DO UPDATE SET 
    weight = $4,
    performance_score = $5,
    sample_size = $6,
    updated_at = NOW()
RETURNING *;

-- name: GetStrategyWeight :one
SELECT * FROM strategy_weights 
WHERE strategy_name = $1 
  AND language = $2 
  AND pattern_type = $3;

-- name: GetStrategyWeightsByLanguage :many
SELECT * FROM strategy_weights 
WHERE language = $1 
ORDER BY performance_score DESC;

-- name: GetOptimalStrategies :many
SELECT * FROM strategy_weights 
WHERE language = $1 
  AND pattern_type = $2 
  AND sample_size >= $3 
ORDER BY weight DESC, performance_score DESC;

-- name: GetAllStrategyWeights :many
SELECT * FROM strategy_weights 
ORDER BY language, strategy_name;