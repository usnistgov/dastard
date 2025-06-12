package off

import (
	"encoding/json"
	"os"
	"testing"

	"gonum.org/v1/gonum/mat"
)

// func matPrint(X mat.Matrix, t *testing.T) {
// 	fa := mat.Formatted(X, mat.Prefix(""), mat.Squeeze())
// 	t.Logf("%v\n", fa)
// 	fmt.Println(fa)
// }

func TestOff(t *testing.T) {

	// assign the projectors and basis
	nbases := 3
	nsamples := 4
	projectors := mat.NewDense(nbases, nsamples,
		[]float64{1.124, 0, 1.124, 0,
			0, 1, 0, 0,
			0, 0, 1, 0})
	basis := mat.NewDense(nsamples, nbases,
		[]float64{1, 0, 0,
			0, 1, 0,
			0, 0, 1,
			0, 0, 0})

	const maxpre = 100
	const maxsamp = 200
	w := NewWriter("off_test.off", 0, "chan1", 1, maxpre, maxsamp, 9.6e-6, projectors, basis, "dummy model for testing",
		"DastardVersion Placeholder", "GitHash Placeholder", "SourceName Placeholder", TimeDivisionMultiplexingInfo{},
		PixelInfo{}, nil, nil)
	if err := w.CreateFile(); err != nil {
		t.Fatal(err)
	}
	if w.headerWritten {
		t.Error("headerWritten should be false, have", w.headerWritten)
	}
	if err := w.WriteHeader(); err != nil {
		t.Error(err)
	}
	if !w.headerWritten {
		t.Error("headerWritten should be true, have", w.headerWritten)
	}
	if err := w.WriteHeader(); err == nil {
		t.Errorf("expect error from writing header again")
	}
	w.Flush()
	stat, _ := os.Stat("off_test.off")
	sizeHeader := stat.Size()
	if err := w.WriteRecord(0, 0, 123456, 0, 0, 0, .123456, make([]float32, 3)); err != nil {
		t.Error(err)
	}
	w.Flush()
	stat, _ = os.Stat("off_test.off")
	expectSize := sizeHeader + 36 + 4*3
	if stat.Size() != expectSize {
		t.Errorf("wrong size, want %v, have %v", expectSize, stat.Size())
	}
	if w.recordsWritten != 1 {
		t.Error("wrong number of records written, want 1, have", w.recordsWritten)
	}
	if err := w.WriteRecord(0, 0, 0, 0, 0, 0, 0, make([]float32, 10)); err == nil {
		t.Error("should have complained about wrong number of bases")
	}
	w.Close()
	if w.RecordsWritten() != w.recordsWritten {
		t.Error()
	}
	if w.HeaderWritten() != w.headerWritten {
		t.Error()
	}

	// Check the OFF file contents, at least certain header info
	fp, err := os.Open("off_test.off")
	if err != nil {
		t.Error("Could not open off test file")
	}
	defer fp.Close()
	offtext := make([]byte, 8192)
	_, err = fp.Read(offtext)
	if err != nil {
		t.Error("Could not read off test file")
	}
	// Scan through the text read, and stop before first non-printing ASCII byte.
	for i, b := range offtext {
		if b > 127 {
			offtext = offtext[:i]
			break
		}
	}
	var x Writer
	json.Unmarshal(offtext, &x)
	if x.MaxPresamples != maxpre {
		t.Errorf("OFF file says MaxPresamples=%d, want %d", x.MaxPresamples, maxpre)
	}
	if x.MaxSamples != maxsamp {
		t.Errorf("OFF file says MaxPresamples=%d, want %d", x.MaxSamples, maxsamp)
	}
}
