package mysql

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type mySQLConnection struct {
	db         *sql.DB
	datarunmsg chan *DatarunMessage
	// datafilemsg chan datafileMessage
}

var oncedbconn sync.Once
var singledbconn *mySQLConnection // singleton object. DON'T USE outside this source file

func (conn *mySQLConnection) Close() {
	if conn.db != nil {
		conn.db.Close()
	}
}

type DatarunMessage struct {
	Directory    string
	Numchan      int
	Channelgroup string
	Intention    string
	DataSource   string
	Creator      string
	LJH          bool
	OFF          bool
}

type DatafileMessage struct {
}

func RecordDatarun(msg *DatarunMessage) {
	if singledbconn == nil || singledbconn.datarunmsg == nil {
		return
	}
	go func() {
		singledbconn.datarunmsg <- msg
	}()
}

func (conn *mySQLConnection) handleDRMessage(msg *DatarunMessage) {
	fmt.Println("I have received a DataRun message:")
	fmt.Println(*msg)
	if conn.db == nil {
		return
	}

	// Put intention, creator, and channel group info into that table (and get the IDs)
	// Ignore DB errors here, b/c we have default values to use
	const unknownID = int(1)
	insertUniqueGetID := func(table, field, value string) int {
		if len(value) == 0 {
			return unknownID
		}
		qinsert := fmt.Sprintf("INSERT INTO %s(%s) VALUES(?) ON DUPLICATE KEY UPDATE id=id", table, field)
		conn.db.Exec(qinsert, value)
		id := unknownID
		qgetid := fmt.Sprintf("SELECT id FROM %s WHERE %s = ?", table, field)
		fmt.Println(qinsert)
		fmt.Println(qgetid)
		conn.db.QueryRow(qgetid, value).Scan(&id)
		fmt.Printf("%s '%s' is registered in the DB as %s.id=%d\n", field, value, table, id)
		return id
	}
	intentionID := insertUniqueGetID("intentions", "intention", msg.Intention)
	changroupID := insertUniqueGetID("channelgroups", "description", msg.Channelgroup)
	dsourceID := insertUniqueGetID("datasources", "source", msg.DataSource)
	creatorID := insertUniqueGetID("creators", "creator", msg.Creator)
	ljhID := unknownID
	offID := unknownID
	if msg.LJH {
		ljhID = creatorID
	}
	if msg.OFF {
		offID = creatorID
	}

	fmt.Println(intentionID, changroupID, creatorID, dsourceID)

	q := "INSERT INTO dataruns(directory,numchan,channelgroup_id,intention_id,datasource_id,ljhcreator_id,offcreator_id) VALUES(?,?,?,?,?,?,?)"
	conn.db.Exec(q, msg.Directory, msg.Numchan, changroupID, intentionID, dsourceID, ljhID, offID)
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

			rmsg := make(chan *DatarunMessage)
			// fmsg := make(chan datafileMessage)
			singledbconn = &mySQLConnection{db, rmsg} // , fmsg}
		})
}

func PingMySQLServer() bool {
	newMySQLConnection()
	if singledbconn.db == nil {
		fmt.Println("Could not open DB connection")
		return false
	}
	if err := singledbconn.db.Ping(); err != nil {
		fmt.Println("Could not ping the open DB connection:", err)
		return false
	}
	fmt.Println("Ping to MySQL server succeeds.")
	return true
}

func StartMySQLConnection(abort <-chan struct{}) {
	newMySQLConnection()
	go handleConnection(abort)
}

func handleConnection(abort <-chan struct{}) {
	for {
		select {
		case <-abort:
			singledbconn.Close()
			return
		case rmsg := <-singledbconn.datarunmsg:
			singledbconn.handleDRMessage(rmsg)
			// case fmsg := <-connection.datafilemsg:
			//
		}
	}
}
