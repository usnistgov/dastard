package dastard

import (
	"os"
	"strings"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func TestTriggerCoupling(t *testing.T) {
	ds := AnySource{nchan: 8}
	ds.PrepareChannels()
	ds.PrepareRun(100, 1000)
	var err error
	if err = ds.SetCoupling(NoCoupling); err != nil {
		t.Errorf("ds.SetCoupling(NoCoupling) should be allowed")
	}
	if err = ds.SetCoupling(FBToErr); err == nil {
		t.Errorf("ds.SetCoupling(FBToErr) should not be allowed (for non-Lancero source)")
	}

	// Make a GTS object with 5 connections
	connections := make(map[int][]int)
	connections[1] = []int{2, 3, 4}
	connections[5] = []int{6, 7}
	gts := &GroupTriggerState{Connections: connections}

	// Turn on the 5 connections and check some of them.
	ds.ChangeGroupTrigger(true, gts)
	if ds.broker.nconnections != 5 {
		t.Errorf("Broker has %d connections, want 5", ds.broker.nconnections)
	}
	for _, rx := range connections[1] {
		if !ds.broker.isConnected(1, rx) {
			t.Errorf("Broker 1->%d not connected (%v)", rx, ds.broker.sources)
		}
	}

	// Turn them on again; should not be an error.
	ds.ChangeGroupTrigger(true, gts)
	if ds.broker.nconnections != 5 {
		t.Errorf("Broker has %d connections, want 5", ds.broker.nconnections)
	}

	// Turn off the 5 connections and check some of them.
	ds.ChangeGroupTrigger(false, gts)
	if ds.broker.nconnections != 0 {
		t.Errorf("Broker has %d connections, want 0", ds.broker.nconnections)
	}
	for _, rx := range connections[1] {
		if ds.broker.isConnected(1, rx) {
			t.Errorf("Broker 1->%d is still connected (%v)", rx, ds.broker.sources)
		}
	}

	// Turn them off again; should not be an error.
	ds.ChangeGroupTrigger(false, gts)
	if ds.broker.nconnections != 0 {
		t.Errorf("Broker has %d connections, want 0", ds.broker.nconnections)
	}
}

func TestChannelNames(t *testing.T) {
	ds := AnySource{nchan: 4}
	ds.PrepareChannels()
	expect := []string{"chan0", "chan1", "chan2", "chan3"}
	if len(ds.chanNames) != 4 {
		t.Errorf("ds.chanNames length = %d, want 4", len(ds.chanNames))
	} else {
		for i, n := range ds.chanNames {
			if n != expect[i] {
				t.Errorf("ds.chanNames[%d]=%q, want %q", i, n, expect[i])
			}
		}
	}

	row := 3
	col := 4
	nrows := 5
	ncols := 10
	code := rcCode(row, col, nrows, ncols)
	if code.row() != row {
		t.Errorf("rcCode(%d,%d,%d,%d).row() = %d, want %d", row, col, nrows, ncols, code.row(), row)
	}
	if code.col() != col {
		t.Errorf("rcCode(%d,%d,%d,%d).col() = %d, want %d", row, col, nrows, ncols, code.col(), col)
	}
	if code.rows() != nrows {
		t.Errorf("rcCode(%d,%d,%d,%d).rows() = %d, want %d", row, col, nrows, ncols, code.rows(), nrows)
	}
	if code.cols() != ncols {
		t.Errorf("rcCode(%d,%d,%d,%d).cols() = %d, want %d", row, col, nrows, ncols, code.cols(), ncols)
	}
}

func TestWritingFiles(t *testing.T) {
	tmp, err1 := os.MkdirTemp("", "dastardTest")
	if err1 != nil {
		t.Errorf("could not make TempDir")
		return
	}
	defer os.RemoveAll(tmp)

	dir, err2 := makeDirectory(tmp)
	if err2 != nil {
		t.Error(err2)
	} else if !strings.HasPrefix(dir, tmp) {
		t.Errorf("Writing in path %s, which should be a prefix of %s", tmp, dir)
	}
	dir2, err2 := makeDirectory(tmp)
	if err2 != nil {
		t.Error(err2)
	} else if !strings.HasPrefix(dir2, tmp) {
		t.Errorf("Writing in path %s, which should be a prefix of %s", tmp, dir2)
	} else if !strings.HasSuffix(dir2, "run0001_%s.%s") {
		t.Errorf("makeDirectory produces %s, of which %q should be a suffix", dir2, "run0001_%s.%s")
	}

	if _, err := makeDirectory("/notallowed"); err == nil {
		t.Errorf("makeDirectory(%s) should have failed", "/notallowed")
	}

	ds := AnySource{nchan: 4}
	ds.rowColCodes = make([]RowColCode, ds.nchan)
	ds.PrepareChannels()
	ds.PrepareRun(256, 1024)
	defer ds.Stop()
	config := &WriteControlConfig{Request: "Pause", Path: tmp, WriteLJH22: true}

	// set projectors so that we can use WriterOFF = true
	config.WriteOFF = true
	config.Request = "start"
	if err := ds.WriteControl(config); err == nil {
		t.Errorf("expected error for asking to WriteOFF with no projectors set\n%v", config.Request)
	}
	nbases := 1
	nsamples := 1024
	projectors := mat.NewDense(nbases, nsamples, make([]float64, nbases*nsamples))
	basis := mat.NewDense(nsamples, nbases, make([]float64, nbases*nsamples))
	if err1 := ds.processors[0].SetProjectorsBasis(projectors, basis, "test model"); err1 != nil {
		t.Error(err1)
	}
	config.Request = "Start"
	config.WriteLJH22 = true
	config.WriteOFF = true
	config.WriteLJH3 = true
	if err := ds.WriteControl(config); err != nil {
		t.Errorf("%v\n%v", err, config.Request)
	}
	if !ds.processors[0].DataPublisher.HasLJH22() {
		t.Error("WriteLJH22 did not result in HasLJH22")
	}
	if !ds.processors[0].DataPublisher.HasOFF() {
		t.Error("WriteOFF did not result in HasOFF")
	}
	if ds.processors[1].DataPublisher.HasOFF() {
		t.Error("WriteOFF resulting in HasOFF for a channel without projectors")
	}
	if !ds.processors[0].DataPublisher.HasLJH3() {
		t.Error("WriteLJH3 did not result in HasLJH3")
	}
	config.Request = "PAUSE"
	if err := ds.WriteControl(config); err != nil {
		t.Errorf("%v\n%v", err, config.Request)
	}
	config.Request = "UnPAUSE "
	if err := ds.WriteControl(config); err == nil {
		t.Errorf("expected error for length==8, %v", config.Request)
	}
	config.Request = "UnPAUSEZZZZ"
	if err := ds.WriteControl(config); err == nil {
		t.Errorf("expected error for 8th character not equal to a space, %v", config.Request)
	}
	config.Request = "UnPAUSE AQ7"
	if err := ds.WriteControl(config); err != nil {
		t.Error(err)
	}
	experimentStateFilename := ds.writingState.ExperimentStateFilename
	config.Request = "Stop"
	if err := ds.WriteControl(config); err != nil {
		t.Errorf("%v\n%v", err, config.Request)
	}
	if ds.processors[0].DataPublisher.HasLJH22() {
		t.Error("Stop did not result in !HasLJH22")
	}
	if ds.processors[0].DataPublisher.HasOFF() {
		t.Error("Stop did not result in !HasOFF")
	}
	if ds.processors[0].DataPublisher.HasLJH3() {
		t.Error("Stop did not result in !HasLJH3")
	}

	fileContents, err2 := os.ReadFile(experimentStateFilename)
	fileContentsStr := string(fileContents)
	if err2 != nil {
		t.Error(err2)
	}
	expectFileContentsStr := "# unix time in nanoseconds, state label\n1538424162462127037, START\n1538174046828690465, AQ7\n1538424428433771969, STOP\n"
	if !strings.HasPrefix(fileContentsStr, "# unix time in nanoseconds, state label\n") ||
		!strings.Contains(fileContentsStr, ", START\n") ||
		!strings.Contains(fileContentsStr, ", AQ7\n") ||
		!strings.HasSuffix(fileContentsStr, ", STOP\n") ||
		len(expectFileContentsStr) != len(fileContentsStr) {
		t.Errorf("have\n%v\nwant (except timestamps should disagree)\n%v\n", fileContentsStr, expectFileContentsStr)
	}
}
