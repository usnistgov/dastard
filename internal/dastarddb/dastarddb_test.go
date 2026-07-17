package dastarddb

import (
	"database/sql"
	"runtime"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestDB returns a fully initialized, isolated database for a single test.
func setupTestDB(t *testing.T) *DastardDBConnection {
	// ":memory:" tells SQLite to keep the entire database in RAM.
	// It is blazing fast and never touches your hard drive.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test db: %v", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	testdb := &DastardDBConnection{db: db}

	// t.Cleanup registers a function to run when the test completes.
	// This ensures the database is safely closed even if the test panics or fails,
	// saving you from having to remember `defer logger.Close()` in every test.
	t.Cleanup(func() {
		testdb.Close()
	})

	return testdb
}

func TestDB_Insert(t *testing.T) {
	// Insert a line with plain SQL, multiple times. Make sure it
	db := setupTestDB(t)

	hostname := "testhost.local"
	git_hash := "1234567"
	nEntries := 2

	for range nEntries {
		if _, err := db.db.Exec("INSERT INTO activity (hostname, git_hash) VALUES (?,?)", hostname, git_hash); err != nil {
			t.Errorf("failed to seed database: %v", err)
		}
	}
	var hn string
	var gh string
	err := db.db.QueryRow("SELECT git_hash, hostname FROM activity WHERE id = ?", nEntries).Scan(&gh, &hn)
	if err != nil {
		t.Fatalf("failed to query git_hash: %v", err)
	}

	if gh != git_hash {
		t.Errorf("expected git_hash '%s', got '%s'", git_hash, gh)
	}
	if hn != hostname {
		t.Errorf("expected hostname '%s', got '%s'", hostname, hn)
	}

	var n int
	err = db.db.QueryRow("SELECT count(*) FROM activity").Scan(&n)
	if err != nil {
		t.Fatalf("failed to query count(*): %v", err)
	}

	if n != nEntries {
		t.Errorf("expected count %d, got %d", nEntries, n)
	}
}

func TestDB_LogDastardActivity(t *testing.T) {
	// Insert a line with plain SQL, multiple times. Make sure it
	db := setupTestDB(t)
	msg := &DastardActivityMessage{
		Hostname:  "testhost.local",
		Githash:   "1234567",
		Builddate: "today",
		Version:   "1.3.55",
		GoVersion: runtime.Version(),
		CPUs:      1000,
		Start:     time.Now(),
	}
	err := db.LogDastardActivity(msg)
	if err != nil {
		t.Errorf("could not run LogDastardActivity: %v", err)
	}

	var dbmsg DastardActivityMessage
	err = db.db.QueryRow("SELECT hostname, version, go_version, numCPUs FROM activity WHERE id = 1").Scan(
		&dbmsg.Hostname, &dbmsg.Version, &dbmsg.GoVersion, &dbmsg.CPUs)
	if err != nil {
		t.Fatalf("failed to query activity: %v", err)
	}
	if msg.Hostname != dbmsg.Hostname {
		t.Errorf("expected hostname '%s', got '%s'", msg.Hostname, dbmsg.Hostname)
	}
}

func TestDB_LogDatarun(t *testing.T) {
	// Insert a line with plain SQL, multiple times. Make sure it
	db := setupTestDB(t)
	msg := DatarunMessage{
		NumChan:    6,
		NumPresamp: 500,
		NumSamples: 1000,
		Users:      "Marie Curie",
		Start:      time.Now(),
	}
	err := db.LogDatarun(&msg)
	if err != nil {
		t.Errorf("could not run LogDatarun: %v", err)
	}

	var dbmsg DatarunMessage
	var start int64
	err = db.db.QueryRow("SELECT number_channels, number_presamp, number_samples, users, run_start FROM dataruns WHERE id = 1").Scan(
		&dbmsg.NumChan, &dbmsg.NumPresamp, &dbmsg.NumSamples, &dbmsg.Users, &start)
	dbmsg.Start = time.UnixMicro(start)
	if err != nil {
		t.Fatalf("failed to query activity: %v", err)
	}
	// Careful: the time.Now() includes monotonic clock info; value retrieved from the DB doesn't.
	// Therefore, must compare using t1.Equal(t2) (truncating t1 to µs) and then copy over the value from the DB.
	if !msg.Start.Truncate(time.Microsecond).Equal(dbmsg.Start) {
		t.Errorf("expected run_start %v, got %v", msg.Start, dbmsg.Start)
	}
	dbmsg.Start = msg.Start
	if dbmsg != msg {
		t.Errorf("expected data '%v', got '%v'", msg, dbmsg)
	}
}
