CREATE TABLE jobs_m3 (
 id TEXT PRIMARY KEY,
 version INTEGER NOT NULL CHECK(version = 1),
 state TEXT NOT NULL CHECK(state IN ('queued','inspecting','downloading','pausing','paused','retry_wait','needs_attention','verifying','completed','failed','cancelled')),
 generation INTEGER NOT NULL CHECK(generation >= 0),
 durable INTEGER NOT NULL CHECK(durable >= 0),
 record BLOB NOT NULL
);
INSERT INTO jobs_m3 SELECT * FROM jobs;
DROP TABLE jobs;
ALTER TABLE jobs_m3 RENAME TO jobs;
PRAGMA user_version = 2;
