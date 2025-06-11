// Package dastarddb provides classes that read or write to a ClickHouse database.
package dastarddb

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/oklog/ulid/v2"
	"github.com/usnistgov/dastard"
)

type dber interface {
	PingServer()
	IsConnected() bool
	Disconnect()
	Wait()
	logActivity()
	handleConnection(<-chan struct{})
}

type DastardDBConnection struct {
	conn          clickhouse.Conn
	err           error
	abort         <-chan struct{}
	activityEntry DastardActivityMessage
	sync.WaitGroup
}

const databaseName = "dastard" // official SQL name of the database

func (db *DastardDBConnection) IsConnected() bool {
	return (db != nil) && (db.conn != nil) && (db.err == nil)
}

func PingServer() error {
	db := createDBConnection()
	if !db.IsConnected() {
		return fmt.Errorf("Database is not connected")
	}
	v, err := db.conn.ServerVersion()
	if err != nil {
		return err
	}
	fmt.Printf("ClickHouse server is alive. Version:\n%s\n", v)
	db.conn.Close()
	return nil
}

func StartDBConnection(abort <-chan struct{}) *DastardDBConnection {
	conn := createDBConnection()
	conn.logActivity()
	go conn.handleConnection(abort)
	return conn
}

func DummyDBConnection() *DastardDBConnection {
	db := &DastardDBConnection{}
	return db
}

func createDBConnection() *DastardDBConnection {
	activityEntry := DastardActivityMessage{
		ID:        ulid.Make().String(),
		Hostname:  dastard.Build.Host,
		Githash:   dastard.Build.Githash,
		Version:   dastard.Build.Version,
		GoVersion: runtime.Version(),
		CPUs:      runtime.NumCPU(),
		Start:     time.Now(),
	}

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
	ctx := context.Background()
	conn, err := clickhouse.Open(&opt)
	if err != nil {
		db.err = err
		return db
	}
	db.conn = conn
	db.activityEntry = activityEntry
	db.Add(1)

	// Ping the server at the DB connection.
	if err = conn.Ping(ctx); err != nil {
		if exception, ok := err.(*clickhouse.Exception); ok {
			fmt.Printf("Exception [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		}
		db.err = err
		return db
	}

	// db.SetConnMaxLifetime(time.Minute * 3)
	// db.SetMaxOpenConns(10)
	// db.SetMaxIdleConns(10)

	// rmsg := make(chan *DatarunMessage)
	// fmsg := make(chan *DatafileMessage)
	// runIDmap := make(map[string]int64)
	return db
}

func (db *DastardDBConnection) logActivity() {
	if !db.IsConnected() {
		return
	}
	batch, err := db.conn.PrepareBatch(context.Background(), "INSERT INTO dastardactivity")
	if err != nil {
		db.err = err
		return
	}
	defer batch.Close()
	ae := db.activityEntry
	err = batch.Append(
		ae.ID, ae.Hostname, ae.Githash, ae.Version,
		ae.GoVersion, ae.CPUs, ae.Start, ae.End)
	if err != nil {
		fmt.Println("Error raised on batch.Append! ", err)
		db.err = err
	}
	err = batch.Send()
	if err != nil {
		fmt.Println("Error raised on batch.Send! ", err)
		db.err = err
	}
}

func (db *DastardDBConnection) handleConnection(abort <-chan struct{}) {
	for {
		select {
		case <-abort:
			db.Disconnect()
			return
			// case rmsg := <-singledbconn.datarunmsg:
			// 	singledbconn.handleDRMessage(rmsg)
			// case fmsg := <-singledbconn.datafilemsg:
			// 	singledbconn.handleDFMessage(fmsg)
		}
	}
}

func (db *DastardDBConnection) Disconnect() {
	db.activityEntry.End = time.Now()
	db.logActivity()
	db.Done()
}
