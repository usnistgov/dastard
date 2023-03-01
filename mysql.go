package dastard

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type datarunMessage struct {
	creator      string
	intention    string
	channelgroup string
}

type datafileMessage struct {
}

type mySQLConnection struct {
	db         *sql.DB
	datarunmsg chan datarunMessage
	// datafilemsg chan datafileMessage
}

var oncedbconn sync.Once
var singledbconn *mySQLConnection // singleton object. DON'T USE outside this source file

func (conn *mySQLConnection) Close() {
	if conn.db != nil {
		conn.db.Close()
	}
}

func (conn *mySQLConnection) handleDRMessage(msg datarunMessage) {
	fmt.Println("I have received a DataRun message:")
	fmt.Print(msg)
}

func newMySQLConnection() {
	if singledbconn != nil {
		return
	}
	oncedbconn.Do(
		func() {
			const sqlType = "mysql"
			const sqlDatabase = "spectrometer"
			sqlUsn := os.Getenv("DASTARD_MYSQL_USER")
			sqlPass := os.Getenv("DASTARD_MYSQL_PASSWORD")
			dataSourceName := fmt.Sprintf("%s:%s@tcp(127.0.0.1:3306)/%s", sqlUsn, sqlPass, sqlDatabase)
			db, err := sql.Open(sqlType, dataSourceName)
			if err != nil {
				return
			}

			db.SetConnMaxLifetime(time.Minute * 3)
			db.SetMaxOpenConns(10)
			db.SetMaxIdleConns(10)

			rmsg := make(chan datarunMessage)
			// fmsg := make(chan datafileMessage)
			singledbconn = &mySQLConnection{db, rmsg} // , fmsg}
		})
}

func closeMySQLConnection() {
	singledbconn.Close()
}

func PingMySQLServer() {
	newMySQLConnection()
	if singledbconn.db == nil {
		fmt.Println("Could not open DB connection")
		return
	}
	if err := singledbconn.db.Ping(); err != nil {
		fmt.Println("Could not ping the open DB connection:", err)
		return
	}
	fmt.Println("Ping to MySQL server succeeds.")
}

func StartMySQLConnection(abort chan struct{}) {
	newMySQLConnection()

	for {
		select {
		case <-abort:
			return
		case rmsg := <-singledbconn.datarunmsg:
			singledbconn.handleDRMessage(rmsg)
			// case fmsg := <-connection.datafilemsg:
			//
		}
	}
}
