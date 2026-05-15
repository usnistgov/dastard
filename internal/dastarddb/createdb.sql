-- Create the Dastard bookkeeping database.

-- To run from the terminal:
-- clickhouse client -u $DASTARD_DB_USER --password $DASTARD_DB_PASSWORD --queries-file createdb.sql
-- Obviously, `export DASTARD_DB_USER=username` and the password first.

-- To completely remove the database before running this program, your query would be
-- DROP DATABASE dastard;
-- USE WITH CAUTION!!!!

CREATE DATABASE IF NOT EXISTS dastard;
USE dastard;

CREATE TABLE IF NOT EXISTS dastardactivity (
    `id`           UUID DEFAULT generateUUIDv7(),
    `hostname`     LowCardinality(String) DEFAULT 'unknown',
    `git_hash`     LowCardinality(String) DEFAULT 'unknown',
    `version`      LowCardinality(String) DEFAULT 'unknown',
    `goversion`    LowCardinality(String) DEFAULT 'unknown',
    `numCPUs`      UInt32 Comment 'Go reports running on this many CPUs',
    `server_start` DateTime64(6),
    `server_end`   DateTime64(6),
)
    ENGINE = ReplacingMergeTree()
    ORDER BY  (toUInt128(id)) -- ClickHouse has a pathological (?) or unsuitable ordering of UUIDs, sorting by the randomness half. This is workaround.
    COMMENT 'Each row represents one invocation of Dastard';


CREATE TABLE IF NOT EXISTS dataruns (
    `id`              UUID DEFAULT generateUUIDv7(),
    `dastard_id`      UUID COMMENT 'link to dastardactivity.id',
    `date_run_code`   String,
    `intention`       LowCardinality(String) DEFAULT 'unknown',
    `datasource`      LowCardinality(String) DEFAULT 'unknown',
    `directory`       String DEFAULT 'unknown',
    `number_channels` UInt32,
    `number_presamp`  UInt32,
    `number_samples`  UInt32,
    `time_offset`     Float64,
    `timebase`        Float64,
    `users`           LowCardinality(String) DEFAULT 'unknown',
    `sample`          LowCardinality(String) DEFAULT 'unknown',
    `purpose`         LowCardinality(String) DEFAULT 'unknown',
    `start`           DateTime64(6),
    `end`             DateTime64(6),
)
    ENGINE = ReplacingMergeTree()
    ORDER BY (toUInt128(id))
    COMMENT 'Each row represents one simultaneous set of Dastard data files';


CREATE TABLE IF NOT EXISTS sensors (
    `id`              UUID DEFAULT generateUUIDv7(),
    `datarun_id`      UUID COMMENT 'link to dataruns.id',
    `date_run_code`   String,
    `row_number`      UInt32 NULL Comment 'TDM row number, or NULL for µMUX',
    `column_number`   UInt32 NULL Comment 'TDM column number, or NULL for µMUX',
    `channel_number`  UInt32,
    `channel_index`   UInt32,
    `channel_name`    String,
    `isError`         Bool DEFAULT false,
)
    ENGINE = MergeTree()
    ORDER BY (toUInt128(id))
    COMMENT 'Each row represents one microcalorimeter in one datarun';

CREATE TABLE IF NOT EXISTS files (
    `sensor_id`       UUID COMMENT 'link to sensors.id',
    `filename`        String,
    `file_type`       LowCardinality(String) DEFAULT 'ljh2',
    `start`           DateTime64(6),
    `end`             DateTime64(6),
    `records`         UInt32,
    `size`            UInt32 COMMENT 'file size in bytes',
    `sha256`          FixedString(64),
)
    ENGINE = ReplacingMergeTree()
    ORDER BY (toUInt128(sensor_id), file_type)
    COMMENT 'Each row represents one microcalorimeter data file';
