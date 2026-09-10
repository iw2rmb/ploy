ALTER TABLE ploy.runs
  ALTER COLUMN status DROP DEFAULT;

---- create above / drop below ----

ALTER TABLE ploy.runs
  ALTER COLUMN status SET DEFAULT 'Queued';
