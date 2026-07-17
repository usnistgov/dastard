package dastarddb

import (
	"database/sql"
	_ "embed"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type DastardDBConnection struct {
	db         *sql.DB
	activityID int64
	datarunID  int64
}

func NewDastardDBConnection(dbPath string) (*DastardDBConnection, error) {
	// 1. Add Pragmas to the connection string
	// - Enable WAL mode
	// - Set a 5-second busy timeout just in case
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// 2. Still keep this! WAL allows concurrent readers,
	// but SQLite still only allows one concurrent writer.
	db.SetMaxOpenConns(1)

	// Execute the embedded file contents
	if _, err := db.Exec(schemaSQL); err != nil {
		log.Fatal("Failed to create tables:", err)
	}

	newval := &DastardDBConnection{db: db}
	return newval, nil
}

func (db *DastardDBConnection) Close() error {
	return db.db.Close()
}

type DastardActivityMessage struct {
	Hostname  string
	Githash   string
	Builddate string
	Version   string
	GoVersion string
	CPUs      int
	Start     time.Time
}

func (db *DastardDBConnection) LogDastardActivity(m *DastardActivityMessage) error {
	start := m.Start.UnixMicro()
	result, err := db.db.Exec(
		"INSERT INTO activity (hostname, git_hash, build_date, version, go_version, numCPUs, server_start, server_seen) VALUES (?,?,?,?,?,?,?,?)",
		m.Hostname, m.Githash, m.Builddate, m.Version, m.GoVersion, m.CPUs, start, start,
	)
	if err == nil {
		// Set up a timer to update the "server_seen" column every minute.
		// It will run until process ends.
		db.activityID, err = result.LastInsertId()
		go func() {
			ticker := time.NewTicker(time.Minute)
			for range ticker.C {
				db.updateActivity()
			}
		}()
	}
	return err
}

func (db *DastardDBConnection) updateActivity() error {
	seen := time.Now().UnixMicro()
	result, err := db.db.Exec("UPDATE activity SET server_seen = ? WHERE id = ?", seen, db.activityID)
	if err != nil {
		log.Println("Error UPDATEing: ", err)
		return err
	}
	nrows, err := result.RowsAffected()
	if err == nil && nrows == 0 {
		log.Printf("Warning: no rows updated for activity.id=%d", db.activityID)
	}
	return err
}

type DatarunMessage struct {
	NumChan    int
	NumPresamp int
	NumSamples int
	Timebase   float64
	DateRun    string
	BasePath   string
	DataSource string
	Intention  string
	Users      string
	Sample     string
	Purpose    string
	Start      time.Time
}

func (db *DastardDBConnection) LogDatarun(m *DatarunMessage) error {
	start := m.Start.UnixMicro()
	result, err := db.db.Exec(
		`INSERT INTO dataruns (activity_id, number_channels, number_presamp, number_samples, 
			timebase, date_run_code, base_storage, intention, datasource, users, sample, purpose, run_start) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		db.activityID, m.NumChan, m.NumPresamp, m.NumSamples, m.Timebase, m.DateRun,
		m.BasePath, m.Intention, m.DataSource, m.Users, m.Sample, m.Purpose, start,
	)
	if err == nil {
		if id, err := result.LastInsertId(); err == nil {
			db.datarunID = id
		}
	}
	return err
}

func (db *DastardDBConnection) EndDatarun(end time.Time) error {
	result, err := db.db.Exec("UPDATE dataruns SET run_end = ? WHERE id = ?", end.UnixMicro(), db.datarunID)
	if err != nil {
		log.Println("Error UPDATEing: ", err)
		return err
	}
	nrows, err := result.RowsAffected()
	if err == nil && nrows == 0 {
		log.Printf("Warning: no rows updated for dataruns ID=%d", db.datarunID)
	}
	return err
}
