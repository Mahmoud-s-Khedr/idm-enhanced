CREATE TABLE jobs (
 id TEXT PRIMARY KEY,
 version INTEGER NOT NULL CHECK(version = 1),
 state TEXT NOT NULL CHECK(state IN ('queued','inspecting','downloading','pausing','paused','needs_attention','verifying','completed','failed','cancelled')),
 generation INTEGER NOT NULL CHECK(generation >= 0),
 durable INTEGER NOT NULL CHECK(durable >= 0),
 record BLOB NOT NULL
);
PRAGMA user_version = 1;
