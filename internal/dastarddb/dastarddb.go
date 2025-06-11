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

type DastardDBConnection struct {
	conn clickhouse.Conn
	err  error
	sync.WaitGroup
}

var singledbconn *DastardDBConnection // singleton object, not used outside this source file
var oncedbconn sync.Once              // this Once object ensures we initialize the singledbconn only once
const databaseName = "dastard"        // official SQL name of the database

var activityEntry DastardActivityMessage

func IsConnected() bool {
	return (singledbconn.err == nil) && (singledbconn.conn != nil)
}

func StartDBConnection(abort <-chan struct{}) {
	oncedbconn.Do(createDBConnection)
	logActivity()
	go handleConnection(abort)
}

func createDBConnection() {
	activityEntry = DastardActivityMessage{
		ID:        ulid.Make().String(),
		Hostname:  dastard.Build.Host,
		Githash:   dastard.Build.Githash,
		Version:   dastard.Build.Version,
		GoVersion: runtime.Version(),
		CPUs:      runtime.NumCPU(),
		Start:     time.Now(),
	}

	singledbconn = &DastardDBConnection{
		conn: nil,
		err:  nil,
	}
	singledbconn.Add(1)
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
		singledbconn.err = err
		return
	}
	singledbconn.conn = conn
	if err = conn.Ping(ctx); err != nil {
		if exception, ok := err.(*clickhouse.Exception); ok {
			fmt.Printf("Exception [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		}
		singledbconn.err = err
		return
	}

	// db.SetConnMaxLifetime(time.Minute * 3)
	// db.SetMaxOpenConns(10)
	// db.SetMaxIdleConns(10)

	// rmsg := make(chan *DatarunMessage)
	// fmsg := make(chan *DatafileMessage)
	// runIDmap := make(map[string]int64)
}

func logActivity() {
	if !IsConnected() {
		return
	}
	batch, err := singledbconn.conn.PrepareBatch(context.Background(), "INSERT INTO dastardactivity")
	if err != nil {
		singledbconn.err = err
		return
	}
	defer batch.Close()
	err = batch.Append(
		activityEntry.ID, activityEntry.Hostname, activityEntry.Githash, activityEntry.Version,
		activityEntry.GoVersion, activityEntry.CPUs, activityEntry.Start, activityEntry.End)
	if err != nil {
		fmt.Println("Error raised on batch.Append! ", err)
		singledbconn.err = err
	}
	err = batch.Send()
	if err != nil {
		fmt.Println("Error raised on batch.Send! ", err)
		singledbconn.err = err
	}
}

func handleConnection(abort <-chan struct{}) {
	for {
		select {
		case <-abort:
			Disconnect()
			return
			// case rmsg := <-singledbconn.datarunmsg:
			// 	singledbconn.handleDRMessage(rmsg)
			// case fmsg := <-singledbconn.datafilemsg:
			// 	singledbconn.handleDFMessage(fmsg)
		}
	}
}

func Disconnect() {
	activityEntry.End = time.Now()
	logActivity()
	singledbconn.Done()
}

func Wait() {
	if IsConnected() {
		singledbconn.Wait()
		singledbconn.conn = nil
	}
}
