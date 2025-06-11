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
    `id`           FixedString(26),
    `hostname`     LowCardinality(String) DEFAULT 'unknown',
    `git_hash`     LowCardinality(String) DEFAULT 'unknown',
    `version`      LowCardinality(String) DEFAULT 'unknown',
    `goversion`    LowCardinality(String) DEFAULT 'unknown',
    `numCPUs`      UInt32 Comment 'Go reports running on this many CPUs',
    `server_start` DateTime64(6),
    `server_end`   DateTime64(6),
)
    ENGINE = MergeTree()
    PRIMARY KEY (id)
    COMMENT 'Each row represents one invocation of Dastard';

INSERT into dastardactivity (id, hostname, numCPUs, server_start) VALUES
    (generateULID(), 'dummyhostA', 8, now64(6));
INSERT into dastardactivity (id, hostname, numCPUs, server_start) VALUES
    (generateULID(), 'dummyhostB', 12, now64(6));

SELECT * from dastardactivity ORDER BY id;

CREATE TABLE IF NOT EXISTS dataruns (
    `id`              FixedString(26),
    `dastard_id`      FixedString(26),
    `date_run_code`   String,
    `intention`       LowCardinality(String) DEFAULT 'unknown',
    `datasource`      LowCardinality(String) DEFAULT 'unknown',
    `directory`       String DEFAULT 'unknown',
    `number_rows`     UInt32 NULL Comment 'Number of TDM rows, or NULL for µMUX',
    `number_columns`  UInt32 NULL Comment 'Number of TDM columns, or NULL for µMUX',
    `number_channels` UInt32,
    `number_presamp`  UInt32,
    `number_samples`  UInt32,
    `time_offset`     Float64,
    `timebase`        Float64,
    `start`           DateTime64(6),
    `end`             DateTime64(6),
)
    ENGINE = MergeTree()
    PRIMARY KEY (id)
    COMMENT 'Each row represents one simultaneous set of Dastard data files';

INSERT into dataruns (id, date_run_code, intention, datasource, number_channels, timebase, start) VALUES
    (generateULID(), '20250610/0000', 'noise', 'abaco', 16, 16.384e-6, now64(6));


CREATE TABLE IF NOT EXISTS sensors (
    `id`              FixedString(26),
    `datarun_id`      FixedString(26),
    `date_run_code`   String,
    `row_number`      UInt32 NULL Comment 'TDM row number, or NULL for µMUX',
    `column_number`   UInt32 NULL Comment 'TDM column number, or NULL for µMUX',
    `channel_number`  UInt32,
    `channel_index`   UInt32,
    `channel_name`    String,
    `isError`         Bool,
)
    ENGINE = MergeTree()
    PRIMARY KEY (id)
    COMMENT 'Each row represents one microcalorimeter in one datarun';

CREATE TABLE IF NOT EXISTS files (
    `id`              FixedString(26),
    `sensor_id`       FixedString(26),
    `filename`        String,
    `file_type`       LowCardinality(String) DEFAULT 'ljh2',
    `start`           DateTime64(6),
    `end`             DateTime64(6),
    `records`         UInt32,
    `size_bytes`      UInt32,
    `sha256`          FixedString(64),
)
    ENGINE = MergeTree()
    PRIMARY KEY (id)
    COMMENT 'Each row represents one microcalorimeter data file';
