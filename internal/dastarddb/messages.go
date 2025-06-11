package dastarddb

import "time"

// The composite types used for messages to the ClickHouse database.

// DastardActivityMessage is the information for the dastardactivity table.
type DastardActivityMessage struct {
	ID        string
	Hostname  string
	Githash   string
	Version   string
	GoVersion string
	CPUs      int
	Start     time.Time
	End       time.Time
}

// DatarunMessage is the information required to make an entry in the dataruns table.
type DatarunMessage struct {
	ID          string
	DastardID   string
	DataRunCode string
	Intention   string
	DataSource  string
	Directory   string
	isTDM       bool
	Nrows       int
	Ncols       int
	Nchannels   int
	NPresamples int
	NSamples    int
	TimeOffset  time.Time
	Timebase    float64
	Start       time.Time
	End         time.Time
}
