CREATE TABLE jobs_m7 (
 id TEXT PRIMARY KEY,
 version INTEGER NOT NULL CHECK(version = 1),
 state TEXT NOT NULL CHECK(state IN ('queued','inspecting','analyzing','downloading','pausing','paused','retry_wait','needs_attention','verifying','assembling','completed','failed','cancelled')),
 generation INTEGER NOT NULL CHECK(generation >= 0),
 durable INTEGER NOT NULL CHECK(durable >= 0),
 record BLOB NOT NULL
);
INSERT INTO jobs_m7 SELECT * FROM jobs;
DROP TABLE jobs;
ALTER TABLE jobs_m7 RENAME TO jobs;
CREATE TABLE media_checkpoints (job_id TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE, record BLOB NOT NULL CHECK(length(record) <= 4194304));
PRAGMA user_version = 5;
