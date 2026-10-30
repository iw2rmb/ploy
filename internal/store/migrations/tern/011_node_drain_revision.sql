ALTER TABLE ploy.nodes ADD COLUMN drain_revision BIGINT NOT NULL DEFAULT 0;

---- create above / drop below ----

ALTER TABLE ploy.nodes DROP COLUMN drain_revision;
