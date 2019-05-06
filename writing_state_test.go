package dastard

import (
	"io/ioutil"
	"os"
	"testing"
)

// See also data_source_test.go, which contains several implicit tests of WritingState.

func TestWriteControl(t *testing.T) {
	tmp, err1 := ioutil.TempDir("", "dastardTest")
	if err1 != nil {
		t.Errorf("could not make TempDir")
		return
	}
	defer os.RemoveAll(tmp)

	ds := AnySource{nchan: 4}
	ds.rowColCodes = make([]RowColCode, ds.nchan)
	ds.PrepareRun(256, 1024)
	defer ds.Stop()
	config := &WriteControlConfig{Request: "Pause", Path: tmp, WriteLJH22: true}
	for _, request := range []string{"Pause", "Unpause", "Stop"} {
		config.Request = request
		if err := ds.WriteControl(config); err != nil {
			t.Errorf("WriteControl request %s failed on a non-writing file: %v", request, err)
		}
	}
	config.Request = "notvalid"
	if err := ds.WriteControl(config); err == nil {
		t.Errorf("WriteControl request %s should fail, but didn't", config.Request)
	}
	config.Request = "Start"
	config.WriteLJH22 = false
	if err := ds.WriteControl(config); err == nil {
		t.Errorf("WriteControl request Start with no valid filetype should fail, but didn't")
	}
	config.WriteLJH22 = true
	config.Path = "/notvalid/because/permissions"
	if err := ds.WriteControl(config); err == nil {
		t.Errorf("WriteControl request Start with nonvalid path should fail, but didn't")
	}

	config.Path = tmp
	if err := ds.WriteControl(config); err != nil {
		t.Errorf("WriteControl request %s failed: %v", config.Request, err)
	}
	for _, request := range []string{"Pause", "Unpause", "Stop"} {
		config.Request = request
		if err := ds.WriteControl(config); err != nil {
			t.Errorf("WriteControl request %s failed on a writing file: %v", request, err)
		}
	}
}
