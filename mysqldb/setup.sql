-- Create the Dastard files database. Run as
-- mysql -u USERNAME -p$DASTARD_MYSQLPASSWORD < setup.sql
-- Obviously, `export MYSQLPASSWORD=xyz` first.

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

CREATE TABLE IF NOT EXISTS ftypes (
    id INT AUTO_INCREMENT PRIMARY KEY,
    code VARCHAR(64) UNIQUE
);

INSERT INTO ftypes (code) VALUES('LJH');
INSERT INTO ftypes (code) VALUES('OFF');

CREATE TABLE IF NOT EXISTS dataruns (
    id INT AUTO_INCREMENT PRIMARY KEY,
    hostname        VARCHAR(256),
    directory       VARCHAR(256),
    numchan         INT,
    channelgroup_id INT,
    intention_id    INT,
    datasource      VARCHAR(64),
    ljhcreator_id   INT,
    offcreator_id   INT
);

CREATE TABLE IF NOT EXISTS channelgroups (
    id INT AUTO_INCREMENT PRIMARY KEY,
    description VARCHAR(256) UNIQUE
);

DROP TABLE IF EXISTS intentions;
CREATE TABLE IF NOT EXISTS intentions (
    id INT AUTO_INCREMENT PRIMARY KEY,
    intention VARCHAR(64)
);

INSERT INTO intentions (intention) VALUES('unknown');
INSERT INTO intentions (intention) VALUES('noise');
INSERT INTO intentions (intention) VALUES('training');
INSERT INTO intentions (intention) VALUES('pulses');

CREATE TABLE IF NOT EXISTS creators (
    id INT AUTO_INCREMENT PRIMARY KEY,
    creator VARCHAR(256) UNIQUE
);

INSERT INTO creators (creator) VALUES('unknown');
