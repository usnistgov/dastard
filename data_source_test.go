package dastard

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestChannelNames(t *testing.T) {
	ds := AnySource{nchan: 4}
	ds.setDefaultChannelNames()
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
}

func TestWritingFiles(t *testing.T) {
	tmp, err1 := ioutil.TempDir("", "dastardTest")
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
	} else if !strings.HasSuffix(dir2, "run0001_%s.ljh") {
		t.Errorf("makeDirectory produces %s, of which %q should be a suffix", dir2, "run0001_%s.ljh")
	}

	if _, err := makeDirectory("/notallowed"); err == nil {
		t.Errorf("makeDirectory(%s) should have failed", "/notallowed")
	}

	ds := AnySource{nchan: 4}
	ds.PrepareRun()
	config := &WriteControlConfig{Request: "Pause", Path: tmp, FileType: "LJH2.2"}
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
	config.FileType = "notvalid"
	if err := ds.WriteControl(config); err == nil {
		t.Errorf("WriteControl request Start with nonvalid filetype should fail, but didn't")
	}

	config.FileType = "LJH2.2"
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
