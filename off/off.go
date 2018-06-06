// Package ljh provides classes that write OFF files
package off

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/usnistgov/dastard/getbytes"
	"gonum.org/v1/gonum/mat"
)

// Writer writes OFF files
type Writer struct {
	ChanNum         int
	Timebase        float64
	NumberOfRows    int
	NumberOfColumns int
	NumberOfBases   int
	Row             int
	Column          int
	HeaderWritten   bool
	FileName        string
	RecordsWritten  int
	Projectors      *mat.Dense
	Basis           *mat.Dense

	file   *os.File
	writer *bufio.Writer
}

// HeaderTDM stores info related to tdm readout for printing to the file header
type HeaderTDM struct {
	NumberOfRows    int
	NumberOfColumns int
	Row             int
	Column          int
}

// Header stores info for the file header, and formats the json correctly
// Projectors and Basis are base64 representations of the output of gonum.mat.MarshallBinary
type Header struct {
	Frameperiod      float64   `json:"frameperiod"`
	Format           string    `json:"File Format"`
	FormatVersion    string    `json:"File Format Version"`
	TDM              HeaderTDM `json:"TDM"`
	ProjectorsBase64 string
	BasisBase64      string
}

// WriteHeader writes a header to the file
func (w *Writer) WriteHeader() error {
	if w.HeaderWritten {
		return errors.New("header already written")
	}
	projectorsBytes, err := w.Projectors.MarshalBinary()
	if err != nil {
		return err
	}
	basisBytes, err := w.Basis.MarshalBinary()
	if err != nil {
		return err
	}
	h := Header{Frameperiod: w.Timebase, Format: "OFF", FormatVersion: "1.0.0",
		TDM: HeaderTDM{NumberOfRows: w.NumberOfRows, NumberOfColumns: w.NumberOfColumns,
			Row: w.Row, Column: w.Column},
		ProjectorsBase64: base64.StdEncoding.EncodeToString(projectorsBytes),
		BasisBase64:      base64.StdEncoding.EncodeToString(basisBytes)}
	s, err := json.MarshalIndent(h, "", "    ")
	if err != nil {
		panic("MarshallIndent error")
	}
	if _, err = w.writer.Write(s); err != nil {
		return err
	}
	if _, err = w.writer.WriteString("\n"); err != nil {
		return err
	}
	w.HeaderWritten = true
	return nil
}

// WriteRecord writes a record to the file
func (w *Writer) WriteRecord(firstRisingSample int, rowcount int64, timestamp int64, data []float32) error {
	if len(data) != w.NumberOfBases {
		return fmt.Errorf("wrong number of bases, have %v, want %v", len(data), w.NumberOfBases)
	}
	if _, err := w.writer.Write(getbytes.FromUint32(uint32(len(data)))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt32(int32(firstRisingSample))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt64(rowcount)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt64(timestamp)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromSliceFloat32(data)); err != nil {
		return err
	}
	w.RecordsWritten += 1
	return nil
}

// Flush flushes the write buffer
func (w Writer) Flush() {
	w.writer.Flush()
}

// Close closes the file, it flushes the bufio.Writer first
func (w Writer) Close() {
	w.writer.Flush()
	w.file.Close()
}

// CreateFile creates a file at w.FileName
// must be called before WriteHeader or WriteRecord
func (w *Writer) CreateFile() error {
	if w.file == nil {
		file, err := os.Create(w.FileName)
		if err != nil {
			return err
		}
		w.file = file
	} else {
		return errors.New("file already exists")
	}
	w.writer = bufio.NewWriterSize(w.file, 32768)
	return nil
}
