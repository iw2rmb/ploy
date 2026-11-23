ALTER TABLE ploy.api_tokens
  ADD COLUMN job_id TEXT UNIQUE REFERENCES ploy.jobs(id) ON DELETE CASCADE,
  ADD CONSTRAINT api_tokens_job_role CHECK ((role = 'job') = (job_id IS NOT NULL));

---- create above / drop below ----

DELETE FROM ploy.api_tokens WHERE role = 'job';
ALTER TABLE ploy.api_tokens DROP COLUMN job_id;
