package off

import (
	"fmt"
	"os"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func matPrint(X mat.Matrix, t *testing.T) {
	fa := mat.Formatted(X, mat.Prefix(""), mat.Squeeze())
	t.Logf("%v\n", fa)
	fmt.Println(fa)
}

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

	w := Writer{FileName: "off_test.off", Projectors: projectors, Basis: basis, NumberOfBases: nbases}
	if err := w.CreateFile(); err != nil {
		t.Error(err)
	}
	if w.HeaderWritten {
		t.Error("HeaderWritten should be false, have", w.HeaderWritten)
	}
	if err := w.WriteHeader(); err != nil {
		t.Error(err)
	}
	if !w.HeaderWritten {
		t.Error("HeaderWritten should be true, have", w.HeaderWritten)
	}
	w.Flush()
	stat, _ := os.Stat("off_test.off")
	sizeHeader := stat.Size()
	if err := w.WriteRecord(0, 0, 0, make([]float32, 3)); err != nil {
		t.Error(err)
	}
	w.Flush()
	stat, _ = os.Stat("off_test.off")
	expectSize := sizeHeader + 8 + 8 + 8 + 3*4
	if stat.Size() != expectSize {
		t.Errorf("wrong size, want %v, have %v", expectSize, stat.Size())
	}
	if w.RecordsWritten != 1 {
		t.Error("wrong number of records written, want 1, have", w.RecordsWritten)
	}
	if err := w.WriteRecord(0, 0, 0, make([]float32, 10)); err == nil {
		t.Error("should have complained about wrong number of bases")
	}
	w.Close()
}
