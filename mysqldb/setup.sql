-- Create the Dastard files database. Run as
-- mysql -u $DASTARD_MYSQL_USER -p$DASTARD_MYSQL_PASSWORD < setup.sql
-- Obviously, `export DASTARD_MYSQL_USER=username` and the password first.

-- To completely remove the database before running this program, your query would be
-- DROP DATABASE spectrometer;
-- USE WITH CAUTION!!!!

CREATE DATABASE IF NOT EXISTS spectrometer;
USE spectrometer;

CREATE TABLE IF NOT EXISTS files (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(256),
    datarun_id INT,
    ftype_id   INT,
    start      TIMESTAMP,
    end        TIMESTAMP,
    size       INT,
    sha256     CHAR(64)
);

DROP TABLE IF EXISTS ftypes;
CREATE TABLE ftypes (
    id INT AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(64) UNIQUE,
    description VARCHAR(256)
);

INSERT INTO ftypes (code, description) VALUES('LJH', 'raw pulse records in LJH 2.2 format');
INSERT INTO ftypes (code, description) VALUES('OFF', 'optimally filtered files in OFF format');

CREATE TABLE IF NOT EXISTS dataruns (
    id INT AUTO_INCREMENT PRIMARY KEY,
    directory       VARCHAR(256),
    numchan         INT,
    channelgroup_id INT,
    intention_id    INT,
    datasource_id   INT,
    ljhcreator_id   INT,
    offcreator_id   INT
);

DROP TABLE IF EXISTS channelgroups;
CREATE TABLE channelgroups (
    id INT AUTO_INCREMENT PRIMARY KEY,
    description VARCHAR(256) UNIQUE
);

INSERT INTO channelgroups(description) VALUES('unknown');

DROP TABLE IF EXISTS intentions;
CREATE TABLE intentions (
    id INT AUTO_INCREMENT PRIMARY KEY,
    intention VARCHAR(64) UNIQUE
);

INSERT INTO intentions (intention) VALUES('unknown');
INSERT INTO intentions (intention) VALUES('noise');
INSERT INTO intentions (intention) VALUES('training');
INSERT INTO intentions (intention) VALUES('pulses');

DROP TABLE IF EXISTS creators;
CREATE TABLE creators (
    id INT AUTO_INCREMENT PRIMARY KEY,
    creator VARCHAR(256) UNIQUE
);

INSERT INTO creators (creator) VALUES('unknown');

DROP TABLE IF EXISTS datasources;
CREATE TABLE datasources (
    id INT AUTO_INCREMENT PRIMARY KEY,
    source VARCHAR(256) UNIQUE
);

INSERT INTO datasources (source) VALUES('unknown');
