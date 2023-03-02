package mysql

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type mySQLConnection struct {
	db          *sql.DB
	datarunmsg  chan *DatarunMessage
	datafilemsg chan *DatafileMessage
}

var oncedbconn sync.Once
var singledbconn *mySQLConnection // singleton object. DON'T USE outside this source file

func (conn *mySQLConnection) Close() {
	if conn.db != nil {
		conn.db.Close()
	}
}

// DatarunMessage is the information required to make an entry in the dataruns table.
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

// DatafileMessage is the information required to make an entry in the files table.
type DatafileMessage struct {
	Filename   string
	Datarun_id int
	Filetype   string
	Starttime  time.Time
}

func RecordDatarun(msg *DatarunMessage) {
	if singledbconn == nil || singledbconn.datarunmsg == nil {
		return
	}
	go func() {
		singledbconn.datarunmsg <- msg
	}()
}

func RecordDatafile(msg *DatafileMessage) {
	if singledbconn == nil || singledbconn.datarunmsg == nil {
		return
	}
	go func() {
		singledbconn.datafilemsg <- msg
	}()
}

const unknownID = int(1)

func insertUniqueGetID(db *sql.DB, table, field, value string) int {
	if len(value) == 0 {
		return unknownID
	}
	qinsert := fmt.Sprintf("INSERT INTO %s(%s) VALUES(?) ON DUPLICATE KEY UPDATE id=id", table, field)
	db.Exec(qinsert, value)
	id := unknownID
	qgetid := fmt.Sprintf("SELECT id FROM %s WHERE %s = ?", table, field)
	fmt.Println(qinsert)
	fmt.Println(qgetid)
	db.QueryRow(qgetid, value).Scan(&id)
	fmt.Printf("%s '%s' is registered in the DB as %s.id=%d\n", field, value, table, id)
	return id
}

func (conn *mySQLConnection) handleDRMessage(msg *DatarunMessage) {
	fmt.Println("I have received a DataRun message:")
	fmt.Println(*msg)
	if conn.db == nil {
		return
	}

	// Put intention, creator, and channel group info into that table (and get the IDs)
	// Ignore DB errors here, b/c we have default values to use
	intentionID := insertUniqueGetID(conn.db, "intentions", "intention", msg.Intention)
	changroupID := insertUniqueGetID(conn.db, "channelgroups", "description", msg.Channelgroup)
	dsourceID := insertUniqueGetID(conn.db, "datasources", "source", msg.DataSource)
	creatorID := insertUniqueGetID(conn.db, "creators", "creator", msg.Creator)
	ljhID := unknownID
	offID := unknownID
	if msg.LJH {
		ljhID = creatorID
	}
	if msg.OFF {
		offID = creatorID
	}

	q := "INSERT INTO dataruns(directory,numchan,channelgroup_id,intention_id,datasource_id,ljhcreator_id,offcreator_id) VALUES(?,?,?,?,?,?,?)"
	conn.db.Exec(q, msg.Directory, msg.Numchan, changroupID, intentionID, dsourceID, ljhID, offID)
}

func (conn *mySQLConnection) handleDFMessage(msg *DatafileMessage) {
	fmt.Println("I have received a Datafile message:")
	fmt.Println(*msg)
	if conn.db == nil {
		return
	}

	ftypeID := insertUniqueGetID(conn.db, "ftypes", "code", msg.Filetype)

	q := "INSERT INTO files(name,ftype_id,start) VALUES(?,?,?)"
	basename := path.Base(msg.Filename)
	start := msg.Starttime.Local().Format("2006-01-02 15:04:05")
	conn.db.Exec(q, basename, ftypeID, start)
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
			fmsg := make(chan *DatafileMessage)
			singledbconn = &mySQLConnection{db, rmsg, fmsg}
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
		case fmsg := <-singledbconn.datafilemsg:
			singledbconn.handleDFMessage(fmsg)
		}
	}
}
