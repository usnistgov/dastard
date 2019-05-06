package dastard

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

// WritingState monitors the state of file writing.
type WritingState struct {
	Active                            bool
	Paused                            bool
	BasePath                          string
	FilenamePattern                   string
	experimentStateFile               *os.File
	ExperimentStateFilename           string
	ExperimentStateLabel              string
	ExperimentStateLabelUnixNano      int64
	ExternalTriggerFilename           string
	externalTriggerNumberObserved     int
	externalTriggerFileBufferedWriter *bufio.Writer
	externalTriggerTicker             *time.Ticker
	externalTriggerFile               *os.File
}

// SetExperimentStateLabel writes to a file with name like XXX_experiment_state.txt
// the file is created upon the first call to this function for a given file writing
func (ws *WritingState) SetExperimentStateLabel(timestamp time.Time, stateLabel string) error {
	if ws.experimentStateFile == nil {
		// create state file if neccesary
		var err error
		ws.experimentStateFile, err = os.Create(ws.ExperimentStateFilename)
		if err != nil {
			return fmt.Errorf("%v, filename: %v", err, ws.ExperimentStateFilename)
		}
		// write header
		_, err1 := ws.experimentStateFile.WriteString("# unix time in nanoseconds, state label\n")
		if err1 != nil {
			return err
		}
	}
	ws.ExperimentStateLabel = stateLabel
	ws.ExperimentStateLabelUnixNano = timestamp.UnixNano()
	_, err := ws.experimentStateFile.WriteString(fmt.Sprintf("%v, %v\n", ws.ExperimentStateLabelUnixNano, stateLabel))
	if err != nil {
		return err
	}
	return nil
}
