package dastard

import (
	"bufio"
	"fmt"
	"os"
	"sync"
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
	sync.Mutex
}

// Start will set the WritingState to begin writing
func (ws *WritingState) Start(filenamePattern, path string) error {
	ws.Lock()
	defer ws.Unlock()
	ws.Active = true
	ws.Paused = false
	ws.BasePath = path
	ws.FilenamePattern = filenamePattern
	ws.ExperimentStateFilename = fmt.Sprintf(filenamePattern, "experiment_state", "txt")
	ws.ExternalTriggerFilename = fmt.Sprintf(filenamePattern, "external_trigger", "bin")
	return ws.setExperimentStateLabel(time.Now(), "START")
}

// Stop will set the WritingState to be completely stopped
func (ws *WritingState) Stop() error {
	ws.Lock()
	defer ws.Unlock()
	ws.Active = false
	ws.Paused = false
	ws.FilenamePattern = ""
	ws.setExperimentStateLabel(time.Now(), "STOP")
	if ws.experimentStateFile != nil {
		if err := ws.experimentStateFile.Close(); err != nil {
			return fmt.Errorf("failed to close experimentStatefile, err: %v", err)
		}
	}
	ws.experimentStateFile = nil
	ws.ExperimentStateFilename = ""
	ws.ExperimentStateLabel = ""
	ws.ExperimentStateLabelUnixNano = 0
	if ws.externalTriggerFile != nil {
		if err := ws.externalTriggerFileBufferedWriter.Flush(); err != nil {
			return fmt.Errorf("failed to flush externalTriggerFileBufferedWriter, err: %v", err)
		}
		if err := ws.externalTriggerFile.Close(); err != nil {
			return fmt.Errorf("failed to close externalTriggerFileWriter, err: %v", err)
		}
		ws.externalTriggerFileBufferedWriter = nil
	}
	ws.externalTriggerNumberObserved = 0
	ws.ExternalTriggerFilename = ""
	return nil
}

// SetExperimentStateLabel writes to a file with name like XXX_experiment_state.txt
// The file is created upon the first call to this function for a given file writing.
// This exported version locks the WritingState object.
func (ws *WritingState) SetExperimentStateLabel(timestamp time.Time, stateLabel string) error {
	ws.Lock()
	defer ws.Unlock()
	return ws.setExperimentStateLabel(timestamp, stateLabel)
}

func (ws *WritingState) setExperimentStateLabel(timestamp time.Time, stateLabel string) error {
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
