package ljh

import (
	"testing"
)

func TestOpen(t *testing.T) {
	fileName := "demo_chan11.ljh"
	r, err := OpenReader(fileName)
	if err != nil {
		t.Error(err)
	}
	if r.VersionNumber != Version2_2 {
		t.Error(`r.VersionNumber != Version2_2`)
	}
}

func TestCannotOpenNonLJH(t *testing.T) {
	fileName := "ljh.go"
	if _, err := OpenReader(fileName); err == nil {
		t.Errorf("Opened non-LJH file '%s' without error, expected error", fileName)
	}

	fileName = "doesnt exist"
	if _, err := OpenReader(fileName); err == nil {
		t.Errorf("Opened non-existent file '%s' without error, expected error", fileName)
	}
}
