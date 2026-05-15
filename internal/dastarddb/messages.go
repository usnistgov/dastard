package dastarddb

import (
	"time"

	"github.com/google/uuid"
)

// The composite types used for messages to the ClickHouse database.

// DastardActivityMessage is the information for the dastardactivity table.
// End won't be filled in until Dastard shuts down.
type DastardActivityMessage struct {
	ID        uuid.UUID
	Hostname  string
	Githash   string
	Version   string
	GoVersion string
	CPUs      int
	Start     time.Time
	End       time.Time
}

// DatarunMessage is the information required to make an entry in the dataruns table.
// End won't be filled in until the data run shuts down.
type DatarunMessage struct {
	ID          uuid.UUID
	DastardID   uuid.UUID
	DateRunCode string
	Intention   string
	DataSource  string
	Directory   string
	Nchannels   int
	NPresamples int
	NSamples    int
	TimeOffset  time.Time
	Timebase    float64
	Start       time.Time
	End         time.Time
}

// SensorMessage is the information required to make an entry in the sensors table.
type SensorMessage struct {
	ID          uuid.UUID
	DatarunID   uuid.UUID
	DateRunCode string
	RowNum      int
	ColNum      int
	ChanNum     int
	ChanIndex   int
	ChanName    string
	IsError     bool
}

// FileMessage is the information required to make an entry in the files table.
// End, Records, Size, and SHA256 can't be filled in until the file closes.
type FileMessage struct {
	SensorID uuid.UUID
	Filename string
	Filetype string
	Start    time.Time
	End      time.Time
	Records  int
	Size     int
	SHA256   string
}
