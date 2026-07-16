CREATE TABLE IF NOT EXISTS activity (
    id INTEGER PRIMARY KEY,
    hostname   TEXT DEFAULT 'unknown',
    git_hash   TEXT DEFAULT 'unknown',
    build_date TEXT DEFAULT 'unknown',
    `version`  TEXT DEFAULT 'unknown',
    go_version TEXT DEFAULT 'unknown',
    numCPUs      INTEGER, -- Go reports running on this many CPUs
    server_start INTEGER, -- Date-time in epoch µs
    server_seen  INTEGER  -- Date-time in epoch µs
);
