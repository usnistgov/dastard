// Package dastarddb provides classes that read or write to a ClickHouse database.
package dastarddb

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// type dber interface {
// 	IsConnected() bool
// 	Disconnect()
// 	Wait()
// 	logActivity()
// 	handleConnection(<-chan struct{})
// }

type DastardDBConnection struct {
	conn          clickhouse.Conn
	err           error
	activityEntry *DastardActivityMessage
	sync.WaitGroup
}

const databaseName = "dastard" // official SQL name of the database

func (db *DastardDBConnection) IsConnected() bool {
	return (db != nil) && (db.conn != nil) && (db.err == nil)
}

// PingServer attempts to create a DB connection, inquire the version number, and disconnect.
// It returns a nil error if that all succeeds.
func PingServer() error {
	db := createDBConnection()
	if !db.IsConnected() {
		return fmt.Errorf("database is not connected")
	}
	v, err := db.conn.ServerVersion()
	if err != nil {
		return err
	}
	fmt.Printf("ClickHouse server is alive. Version:\n%s\n", v)
	db.conn.Close()
	return nil
}

func StartDBConnection(activity *DastardActivityMessage, abort <-chan struct{}) *DastardDBConnection {
	conn := createDBConnection()
	conn.activityEntry = activity
	conn.logActivity()
	go conn.handleConnection(abort)
	return conn
}

func DummyDBConnection() *DastardDBConnection {
	db := &DastardDBConnection{}
	db.Add(1)
	return db
}

func createDBConnection() *DastardDBConnection {

	db := &DastardDBConnection{}
	dbUser := os.Getenv("DASTARD_DB_USER")
	dbPass := os.Getenv("DASTARD_DB_PASSWORD")
	auth := clickhouse.Auth{
		Database: databaseName,
		Username: dbUser,
		Password: dbPass,
	}
	client := clickhouse.ClientInfo{
		Products: []struct {
			Name    string
			Version string
		}{
			{Name: "dastard", Version: "unknown"},
		},
	}
	opt :=
		clickhouse.Options{
			Addr:       []string{"localhost:9000"},
			Auth:       auth,
			ClientInfo: client,
			TLS:        nil,
		}
	conn, err := clickhouse.Open(&opt)
	if err != nil {
		db.err = err
		return db
	}
	db.conn = conn
	db.Add(1)

	// Ping the server at the DB connection.
	ctx := context.Background()
	if err = conn.Ping(ctx); err != nil {
		if exception, ok := err.(*clickhouse.Exception); ok {
			fmt.Printf("Exception [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		}
		db.err = err
		return db
	}

	// Some parameters that we might wish to set in the future?
	// db.SetConnMaxLifetime(time.Minute * 3)
	// db.SetMaxOpenConns(10)
	// db.SetMaxIdleConns(10)

	return db
}

const dontwait = false

// const dowait = true

func (db *DastardDBConnection) asyncInsert(query string, wait bool, args ...any) {
	ctx := context.Background()
	var waitint int // 0 = don't wait for flush, 1 = wait for flush
	if wait {
		waitint = 1
	}
	asyncCtx := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"async_insert":          1,
		"wait_for_async_insert": waitint,
	}))
	if err := db.conn.Exec(asyncCtx, query, args...); err != nil {
		fmt.Println("Error raised on async insert ", err)
		db.err = err
	}
}

func (db *DastardDBConnection) logActivity() {
	if !db.IsConnected() {
		return
	}
	ae := db.activityEntry
	formattedStart := ae.Start.Format("2006-01-02 15:04:05.000000")
	formattedEnd := ae.End.Format("2006-01-02 15:04:05.000000")
	// Unlike most DB entries, the activity ones should be synchronous
	ctx := context.Background()
	db.conn.Exec(ctx, `INSERT INTO dastardactivity VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ae.ID, ae.Hostname, ae.Githash, ae.Version,
		ae.GoVersion, ae.CPUs, formattedStart, formattedEnd,
	)

}

func (db *DastardDBConnection) handleConnection(abort <-chan struct{}) {
	defer db.Done()
	defer db.Disconnect()
	<- abort
}

func (db *DastardDBConnection) Disconnect() {
	if db.IsConnected() {
		db.activityEntry.End = time.Now()
		db.logActivity()
	}
}

// RecordDataRun takes a DatarunMessage and stores it in the DB (if it's open).
// This function will block until the select statement in `handleConnection`
// accepts the message.
// WARNING: Don't change this blocking behavior! It is how we ensure that a datarun
// is entered in the DB before any corresponding calls to `RecordDatafile` begin.
// Without the blocking, there would be a race between the 2 kinds of DB entries,
// and some datafiles would be entered without valid datarun IDs.
func (db *DastardDBConnection) RecordDatarun(msg *DatarunMessage) {
	db.handleDRMessage(msg)
}

func (db *DastardDBConnection) FinishDatarun(msg *DatarunMessage) {
	msg.End = time.Now()
	go func() { db.handleDRMessage(msg) }()
}

func (db *DastardDBConnection) RecordSensors(msgs *[]SensorMessage) {
	go func() { db.handleSensorMessages(msgs) }()
}

func (db *DastardDBConnection) RecordFile(msg *FileMessage) {
	go func() { db.handleFileMessage(msg) }()
}

func (db *DastardDBConnection) RecordBaseline(msg *BaselineMonitorMessage) {
	go func() { db.handleBaselineMessage(msg) }()
}

func (db *DastardDBConnection) handleDRMessage(m *DatarunMessage) {
	if !db.IsConnected() || m == nil {
		return
	}
	formattedStart := m.Start.Format("2006-01-02 15:04:05.000000")
	formattedEnd := m.End.Format("2006-01-02 15:04:05.000000")
	toffset := m.TimeOffset.UnixMicro() / 1e6
	db.asyncInsert(`INSERT INTO dataruns VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, dontwait,
		m.ID, db.activityEntry.ID,
		m.DateRunCode, m.Intention, m.DataSource, m.Directory,
		m.Nchannels, m.NPresamples, m.NSamples, toffset, m.Timebase,
		m.Users, m.Sample, m.Purpose,
		formattedStart, formattedEnd,
	)
}

func (db *DastardDBConnection) handleSensorMessages(messages *[]SensorMessage) {
	if !db.IsConnected() || messages == nil || len(*messages) == 0 {
		return
	}
	ctx := context.Background()
	batch, err := db.conn.PrepareBatch(ctx, "INSERT INTO sensors")
	if err != nil {
		fmt.Println("Error in PrepareBatch: ", err)
		fmt.Println(messages)
		return
	}
	for _, m := range *messages {
		batch.Append(
			m.ID, m.DatarunID, m.DateRunCode, m.RowNum, m.ColNum,
			m.ChanNum, m.ChanIndex, m.ChanName, m.IsError,
		)
	}
	batch.Send()
}

func (db *DastardDBConnection) handleFileMessage(m *FileMessage) {
	if !db.IsConnected() || m == nil {
		return
	}
	formattedStart := m.Start.Format("2006-01-02 15:04:05.000000")
	formattedEnd := m.End.Format("2006-01-02 15:04:05.000000")
	db.asyncInsert(`INSERT INTO files VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, dontwait,
		m.SensorID, m.Filename, m.Filetype, formattedStart, formattedEnd,
		m.Records, m.Size, m.SHA256,
	)
}

func (db *DastardDBConnection) handleBaselineMessage(m *BaselineMonitorMessage) {
	if !db.IsConnected() || m == nil {
		return
	}
	formattedTime := m.Timestamp.Format("2006-01-02 15:04:05")
	db.asyncInsert(`INSERT INTO baseline VALUES (?, ?, ?)`, dontwait,
		m.ChanNum, formattedTime, m.Value,
	)
}
