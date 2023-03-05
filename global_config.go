package dastard

import (
	"log"
	"os"
	"time"
)

// Portnumbers structs can contain all TCP port numbers used by Dastard.
type Portnumbers struct {
	RPC            int
	Status         int
	Trigs          int
	SecondaryTrigs int
	Summaries      int
}

// Ports globally holds all TCP port numbers used by Dastard.
var Ports Portnumbers

const BasePort = 5500

func setPortnumbers(base int) {
	Ports.RPC = base
	Ports.Status = base + 1
	Ports.Trigs = base + 2
	Ports.SecondaryTrigs = base + 3
	Ports.Summaries = base + 4
}

// BuildInfo can contain compile-time information about the build
type BuildInfo struct {
	Version string // version stored in global_config.go
	Githash string // 7-character git commit hash
	Gitdate string // Date/time in the git commit
	Date    string // Date/time of the go build command
	Host    string
	Summary string // A summary to enter into file database
}

// Build is a global holding compile-time information about the build
var Build = BuildInfo{
	Version: "0.3.0",
	Githash: "no git hash entered",
	Gitdate: "no git commit date entered",
	Date:    "no build date entered",
	Host:    "no host found",
	Summary: "DASTARD Version x.y.z (git commit ....... of date time)",
}

// DastardStartTime is a global holding the time init() was run
var DastardStartTime time.Time

// ProblemLogger will log warning messages to a file
var ProblemLogger *log.Logger

func init() {
	setPortnumbers(BasePort)
	DastardStartTime = time.Now()

	// Dastard main program will override this, but at least initialize with a sensible value
	ProblemLogger = log.New(os.Stderr, "", log.LstdFlags)
}
