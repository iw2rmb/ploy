ALTER TABLE ploy.runs
  ALTER COLUMN status SET DEFAULT 'Running';

---- create above / drop below ----

ALTER TABLE ploy.runs
  ALTER COLUMN status SET DEFAULT 'Queued';
