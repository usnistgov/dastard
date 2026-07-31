CREATE TABLE IF NOT EXISTS activity (
    id           INTEGER PRIMARY KEY,
    hostname     TEXT DEFAULT 'unknown',
    git_hash     TEXT DEFAULT 'unknown',
    build_date   TEXT DEFAULT 'unknown',
    `version`    TEXT DEFAULT 'unknown',
    go_version   TEXT DEFAULT 'unknown',
    numCPUs      INTEGER, -- Go reports running on this many CPUs
    server_start INTEGER, -- Date-time in epoch µs
    server_seen  INTEGER  -- Date-time in epoch µs
);

CREATE TABLE IF NOT EXISTS dataruns (
    id              INTEGER PRIMARY KEY,
    activity_id     INTEGER,  -- link to activity.id
    number_channels INTEGER,
    number_presamp  INTEGER,
    number_samples  INTEGER,
    timebase        REAL,
    date_run_code   TEXT,
    base_storage    TEXT,
    intention       TEXT DEFAULT 'unknown',  -- could use a lookup table here
    datasource      TEXT,  -- could use a lookup table here
    users           TEXT,  -- could use a lookup table here
    `sample`        TEXT,  -- could use a lookup table here
    purpose         TEXT,  -- could use a lookup table here
    run_start       INTEGER, -- Date-time in epoch µs
    run_end         INTEGER -- Date-time in epoch µs
);

-- There are columns marked "could use a lookup table here", but I do not currently
-- (31 July 2026) think that this is worth the extra effort. Lookup tables are SQLite's
-- only way of managing low-cardinality strings. I expect these tables to be so small
-- that the storage cost of repeated values is small and not worth working around.
