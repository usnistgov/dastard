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
	datarunIDs  map[string]int64 // Maps directory name to its dataruns.id value
}

var singledbconn *mySQLConnection // singleton object, not used outside this source file
var oncedbconn sync.Once          // this Once.Do ensures we don't initialize the singledbconn twice
const sqlTimestampFormat = "2006-01-02 15:04:05"

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
	Fullpath  string
	Filetype  string
	Starttime time.Time
}

// RecordDataRun takes a DatarunMessage and stores it in the DB (if it's open).
// This function will block until the select statement in `handleConnection`
// accepts the message.
// WARNING: Don't change this blocking behavior! It is how we ensure that a datarun
// is entered in the DB before any corresponding calls to `RecordDatafile` begin.
// Without the blocking, there would be a race between the 2 kinds of DB entries,
// and some datafiles would be entered without valid datarun IDs.
func RecordDatarun(msg *DatarunMessage) {
	if singledbconn == nil || singledbconn.datarunmsg == nil {
		return
	}
	singledbconn.datarunmsg <- msg
}

// RecordDatafile takes a DatafileMessage and stores it in the DB (if it's open).
// This function will NOT block for the datafilemsg channel to be empty and emptied,
// but will put the message on the channel at some later time via a goroutine.
// A new datarun message can (theoretically) arrive before old datafile messages are
// all handled. The map mySQLConnection.datarunIDs should ensure that the correct
// datarun ID is associated with datafiles, whether from the new or old run.
func RecordDatafile(msg *DatafileMessage) {
	if singledbconn == nil || singledbconn.datafilemsg == nil {
		return
	}
	go func() {
		singledbconn.datafilemsg <- msg
	}()
}

const unknownID = int64(1)

// insertUniqueGetID puts `value` into `table.field` in the database `db` if not there
// and (whether new or not) returns the id field for that row.
func insertUniqueGetID(db *sql.DB, table, field, value string) int64 {
	if len(value) == 0 {
		return unknownID
	}
	id := unknownID
	qgetid := fmt.Sprintf("SELECT id FROM %s WHERE %s = ?", table, field)
	if err := db.QueryRow(qgetid, value).Scan(&id); err == nil {
		return id
	}

	// The ON DUPLICATE KEY UPDATE clause will usually be unnecessary because we just SELECTed for this value.
	// But database could theoretically have been changed since previous query, so just to be safe, use it.
	qinsert := fmt.Sprintf("INSERT INTO %s(%s) VALUES(?) ON DUPLICATE KEY UPDATE id=id", table, field)
	if result, err := db.Exec(qinsert, value); err == nil {
		if id, err = result.LastInsertId(); err == nil {
			return id
		}
	}
	return unknownID
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
	conn.datarunIDs[msg.Directory] = unknownID
	if result, err := conn.db.Exec(q, msg.Directory, msg.Numchan, changroupID, intentionID, dsourceID, ljhID, offID); err == nil {
		if datarunID, err := result.LastInsertId(); err == nil {
			conn.datarunIDs[msg.Directory] = datarunID
		}
	}
}

func (conn *mySQLConnection) handleDFMessage(msg *DatafileMessage) {
	fmt.Println("I have received a Datafile message:")
	fmt.Println(*msg)
	if conn.db == nil {
		return
	}

	ftypeID := insertUniqueGetID(conn.db, "ftypes", "code", msg.Filetype)

	q := "INSERT INTO files(name,datarun_id,ftype_id,start) VALUES(?,?,?,?)"
	directory := path.Dir(msg.Fullpath)
	basename := path.Base(msg.Fullpath)
	datarunID := conn.datarunIDs[directory]
	if datarunID == 0 {
		datarunID = unknownID
	}
	start := msg.Starttime.Local().Format(sqlTimestampFormat)
	conn.db.Exec(q, basename, datarunID, ftypeID, start)
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
			runIDmap := make(map[string]int64)
			singledbconn = &mySQLConnection{
				db:          db,
				datarunmsg:  rmsg,
				datafilemsg: fmsg,
				datarunIDs:  runIDmap,
			}
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
