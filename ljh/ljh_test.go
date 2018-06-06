package ljh

import (
	"bufio"
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

func TestWriter(t *testing.T) {
	w := Writer{FileName: "writertest.ljh",
		Samples:    100,
		Presamples: 50}
	err := w.CreateFile()
	if err != nil {
		t.Errorf("file creation error: %v", err)
	}
	if w.RecordsWritten != 0 {
		t.Error("RecordsWritten want 0, have", w.RecordsWritten)
	}
	if w.HeaderWritten {
		t.Error("TestWriter: header written should be false")
	}
	err = w.WriteHeader()
	if !w.HeaderWritten {
		t.Error("TestWriter: header written should be true")
	}
	if err != nil {
		t.Errorf("WriteHeader Error: %v", err)
	}
	data := make([]uint16, 100)
	w.Flush()
	stat, _ := os.Stat("writertest.ljh")
	sizeHeader := stat.Size()
	err = w.WriteRecord(8888888, 127, data)
	if err != nil {
		t.Errorf("WriteRecord Error: %v", err)
	}
	if w.RecordsWritten != 1 {
		t.Error("RecordsWritten want 1, have", w.RecordsWritten)
	}
	w.Flush()
	stat, _ = os.Stat("writertest.ljh")
	sizeRecord := stat.Size()
	expectSize := sizeHeader + 8 + 8 + 2*int64(w.Samples)
	if sizeRecord != expectSize {
		t.Errorf("ljh file wrong size after writing record, want %v, have %v", expectSize, sizeRecord)
	}
	// write a record of incorrect size, check for error
	wrongData := make([]uint16, 101)
	err = w.WriteRecord(0, 0, wrongData)
	if err == nil {
		t.Errorf("WriterTest: should have non-nil Error")
	}
	w.Close()
	r, err := OpenReader("writertest.ljh")
	if err != nil {
		t.Errorf("WriterTest, OpenReader Error: %v", err)
	}
	record, err := r.NextPulse()
	if err != nil {
		t.Errorf("WriterTest, NextPulse Error: %v", err)
	}
	if record.TimeCode != 127 {
		t.Errorf("WriterTest, TimeCode Wrong, have %v, wand %v", record.TimeCode, 127)
	}
	if record.RowCount != 8888888 {
		t.Errorf("WriterTest, RowCount Wrong, have %v, want %v", record.RowCount, 8888888)
	}

}

func TestWriter3(t *testing.T) {
	w := Writer3{FileName: "writertest.ljh3"}
	err := w.CreateFile()
	if err != nil {
		t.Errorf("file creation error: %v", err)
	}
	if w.RecordsWritten != 0 {
		t.Error("RecordsWritten want 0, have", w.RecordsWritten)
	}
	if w.HeaderWritten {
		t.Error("TestWriter: header written should be false")
	}
	err = w.WriteHeader()
	if !w.HeaderWritten {
		t.Error("TestWriter: header written should be true")
	}
	if err != nil {
		t.Errorf("WriteHeader Error: %v", err)
	}
	data := make([]uint16, 100)
	w.Flush()
	stat, _ := os.Stat("writertest.ljh3")
	sizeHeader := stat.Size()
	err = w.WriteRecord(0, 0, 0, data)
	if err != nil {
		t.Errorf("WriteRecord Error: %v", err)
	}
	if w.RecordsWritten != 1 {
		t.Error("RecordsWritten want 1, have", w.RecordsWritten)
	}
	w.Flush()
	stat, _ = os.Stat("writertest.ljh3")
	sizeRecord := stat.Size()
	expectSize := sizeHeader + 4 + 4 + 8 + 8 + 2*int64(len(data))
	if sizeRecord != expectSize {
		t.Errorf("ljh file wrong size after writing record, want %v, have %v", expectSize, sizeRecord)
	}
	// write a record of different length, should work
	otherLengthData := make([]uint16, 101)
	err = w.WriteRecord(0, 0, 0, otherLengthData)
	if err != nil {
		t.Errorf("WriterTest: couldn't write other size")
	}
	w.Flush()
	stat, _ = os.Stat("writertest.ljh3")
	sizeRecord = stat.Size()
	expectSize += 4 + 4 + 8 + 8 + 2*int64(len(otherLengthData))
	if sizeRecord != expectSize {
		t.Errorf("ljh file wrong size after writing record, want %v, have %v", expectSize, sizeRecord)
	}
	w.Close()
}

func BenchmarkLJH22(b *testing.B) {
	w := Writer{FileName: "writertest.ljh",
		Samples:    1000,
		Presamples: 50}
	w.CreateFile()
	w.WriteHeader()
	data := make([]uint16, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := w.WriteRecord(8888888, 127, data)
		if err != nil {
			panic(fmt.Sprint(err))
		}
		b.SetBytes(int64(2 * len(data)))
	}
}
func BenchmarkLJH3(b *testing.B) {
	w := Writer3{FileName: "writertest.ljh"}
	w.CreateFile()
	w.WriteHeader()
	data := make([]uint16, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := w.WriteRecord(0, 0, 0, data)
		if err != nil {
			panic(fmt.Sprint(err))
		}
		b.SetBytes(int64(2 * len(data)))
	}
}
func BenchmarkFileWrite(b *testing.B) {
	f, _ := os.Create("benchmark.ljh")
	data := make([]byte, 2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := f.Write(data)
		if err != nil {
			panic(fmt.Sprint(err))
		}
		b.SetBytes(int64(len(data)))
	}
}
func BenchmarkBufIOWrite(b *testing.B) {
	f, _ := os.Create("benchmark.ljh")
	w := bufio.NewWriterSize(f, 65536)
	defer w.Flush()
	defer f.Close()
	data := make([]byte, 2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := w.Write(data)
		if err != nil {
			panic(fmt.Sprint(err))
		}
		b.SetBytes(int64(len(data)))
	}
}
