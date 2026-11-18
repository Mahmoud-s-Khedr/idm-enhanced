CREATE TABLE preferences (id INTEGER PRIMARY KEY CHECK(id = 1), record BLOB NOT NULL);
PRAGMA user_version = 3;
