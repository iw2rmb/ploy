ALTER TABLE IF EXISTS ploy.api_tokens
  ADD COLUMN IF NOT EXISTS username TEXT;

UPDATE ploy.api_tokens
SET username = description
WHERE role = 'control-plane'
  AND NULLIF(BTRIM(COALESCE(username, '')), '') IS NULL
  AND NULLIF(BTRIM(COALESCE(description, '')), '') IS NOT NULL;
