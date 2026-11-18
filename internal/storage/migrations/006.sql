-- Version barrier: older binaries must not interpret DASH media as file jobs.
-- Existing file checkpoint v1 and HLS media checkpoint v1 are unchanged.
PRAGMA user_version = 6;
