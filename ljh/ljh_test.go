package ljh

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestRead(t *testing.T) {
	fileName := "demo_chan11.ljh"
	r, err := OpenReader(fileName)
	if err != nil {
		t.Error(err)
	}
	defer r.Close()

	if r.VersionNumber != Version2_2 {
		t.Error(`r.VersionNumber != Version2_2`)
	}
	var headertests = []struct {
		name  string
		found int
		want  int
	}{
		{"r.ChanNum", r.ChanNum, 11},
		{"r.Presamples", r.Presamples, 256},
		{"r.Samples", r.Samples, 1024},
		{"r.VersionNumber", int(r.VersionNumber), int(Version2_2)},
		{"r.WordSize", r.WordSize, 2},
		{"r.headerLength", r.headerLength, 1216},
		{"r.recordLength", r.recordLength, 2064},
	}
	for _, ht := range headertests {
		if ht.found != ht.want {
			t.Errorf(`%s = %d, want %d`, ht.name, ht.found, ht.want)
		}
	}

	expectedTC := []int64{1462566784410601, 1462566784420431}
	expectedRC := []int64{25221272465, 25221303185}
	for i, tc := range expectedTC {
		pr, errnp := r.NextPulse()
		if errnp != nil {
			t.Error(errnp)
		}
		if pr.TimeCode != tc {
			t.Errorf("r.NextPulse().TimeCode = %d, want %d", pr.TimeCode, tc)
		}
		if pr.RowCount != expectedRC[i] {
			t.Errorf("r.NextPulse().RowCount = %d, want %d", pr.RowCount, expectedRC[i])
		}
	}
	_, err = r.NextPulse()
	if err == nil {
		t.Errorf("r.NextPulse() works after EOF, want error")
	}
}

func TestCannotOpenNonLJH(t *testing.T) {
	fileName := "ljh.go"
	if _, err := OpenReader(fileName); err == nil {
		t.Errorf("Opened non-LJH file '%s' without error, expected error", fileName)
	}

	fileName = "doesnt exist and can\not exist"
	if _, err := OpenReader(fileName); err == nil {
		t.Errorf("Opened non-existent file '%s' without error, expected error", fileName)
	}
}

func TestBadVersionNumbers(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "ljh_test")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDir)
	tempFile := filepath.Join(tempDir, "t1.ljh")
	// t.Logf("Temp dir: %s\n", tempFile)

	var versiontests = []struct {
		vnum      string
		wanterror bool
	}{
		{"askdjhf", true},
		{"2.2", true},
		{"1.2.3", true},
		{"2.1.0", true},
		{"2.1.1", false},
		{"2.2.0", false},
	}
	for _, vt := range versiontests {
		content := []byte(
			fmt.Sprintf("#LJH Memorial File Format\nSave File Format Version: %s\n#End of Header\n", vt.vnum))
		if err = ioutil.WriteFile(tempFile, content, 0666); err != nil {
			t.Error(err)
		}
		_, err = OpenReader(tempFile)
		if (err != nil) != vt.wanterror {
			t.Errorf(`Version number = %s gives error %v, want %v`, vt.vnum, err, vt.wanterror)
		}
	}

}
