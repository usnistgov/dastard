package mysql

// The composite types used for messages to the MySQL database have to be compiled
// whether the real DB connection is compiled ("mysql.go") or the no-DB mockup is
// used ("nodb_mysql.go"). Use the build tag "nodb" to select the latter.

import "time"

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

// DatafileMessage signifies that a single file has been started or finished (opened or closed).
// It holds the information required to make a new entry in the files table (if Starting=true)
// or to update that entry with the end time and the # of records (if Starting=false).
type DatafileMessage struct {
	Fullpath  string    // Complete file path
	Filetype  string    // File suffix, such as LJH, LJH3, or OFF (ignored if Starting=false)
	Timestamp time.Time // Time of file opening or closing
	Starting  bool      // Is this message about a file being *started*? If false, file is closing.
	Records   int       // How many records were written to the file (ignored if Starting=true).
}
