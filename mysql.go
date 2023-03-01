package dastard

import (
	"database/sql"
	"fmt"
	"os"
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

type MySQLConnection struct {
	db         *sql.DB
	datarunmsg chan datarunMessage
	// datafilemsg chan datafileMessage
}

func (conn *MySQLConnection) Close() {
	conn.db.Close()
}

func (conn *MySQLConnection) handleDRMessage(msg datarunMessage) {
	//
}

func newMySQLConnection() (*MySQLConnection, error) {
	const sqlType = "mysql"
	const sqlDatabase = "spectrometer"
	sqlUsn := os.Getenv("DASTARD_MYSQL_USER")
	sqlPass := os.Getenv("DASTARD_MYSQL_PASSWORD")
	dataSourceName := fmt.Sprintf("%s:%s@tcp(127.0.0.1:3306)/%s", sqlUsn, sqlPass, sqlDatabase)
	db, err := sql.Open(sqlType, dataSourceName)

	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	rmsg := make(chan datarunMessage)
	// fmsg := make(chan datafileMessage)
	connection := &MySQLConnection{db, rmsg} // , fmsg}
	return connection, err
}

func PingMySQLServer() {
	connection, err := newMySQLConnection()
	if err != nil {
		fmt.Println("Could not open DB:", err)
		return
	}
	defer connection.Close()

	if err = connection.db.Ping(); err != nil {
		fmt.Println("Could not ping the open DB connection:", err)
		return
	}
	fmt.Println("Ping to MySQL server succeeds.")
}

func RunMySQLConnection(abort chan struct{}) {
	connection, err := newMySQLConnection()
	if err != nil {
		fmt.Println("Could not open DB:", err)
		return
	}
	defer connection.Close()

	for {
		select {
		case <-abort:
			return
			// case rmsg := <-connection.datarunmsg:
			//
			// case fmsg := <-connection.datafilemsg:
			//
		}
	}
}
