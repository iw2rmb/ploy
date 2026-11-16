ALTER TABLE ploy.nodes ADD COLUMN drain_owner TEXT
  CHECK (drain_owner IS NULL OR (drained AND drain_owner = 'cleanup'));

---- create above / drop below ----

ALTER TABLE ploy.nodes DROP COLUMN drain_owner;
