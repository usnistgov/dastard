// Package ljh provides classes that write OFF files
package off

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
)

// Writer writes OFF files
type Writer struct {
	ChanNum                  int
	Timebase                 float64
	NumberOfRows             int
	NumberOfColumns          int
	NumberOfBases            int
	ProjectorsPlaceHolder    string
	BasisPlaceHolder         string
	NoiseWhitenerPlaceHolder string
	Row                      int
	Column                   int
	HeaderWritten            bool
	FileName                 string
	RecordsWritten           int

	file   *os.File
	writer bufio.Writer
}

// HeaderTDM stores info related to tdm readout for printing to the file header
type HeaderTDM struct {
	NumberOfRows    int
	NumberOfColumns int
	Row             int
	Column          int
}

// Header stores info for the file header, and formats the json correctly
type Header struct {
	Frameperiod   float64   `json:"frameperiod"`
	Format        string    `json:"File Format"`
	FormatVersion string    `json:"File Format Version"`
	TDM           HeaderTDM `json:"TDM"`
}

// WriteHeader writes a header to the file
func (w *Writer) WriteHeader() error {
	if w.HeaderWritten {
		return errors.New("header already written")
	}
	h := Header{Frameperiod: w.Timebase, Format: "OFF", FormatVersion: "1.0.0",
		TDM: HeaderTDM{NumberOfRows: w.NumberOfRows, NumberOfColumns: w.NumberOfColumns,
			Row: w.Row, Column: w.Column}}
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
	if _, err := w.writer.Write(getbytes.FromUint32(uint32(len(data)))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt32(int32(firstRisingSample))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt64(rowcount)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt642(timestamp)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromSliceFloat32(data)); err != nil {
		return err
	}
	w.RecordsWritten += 1
	return nil
}

// Close closes the file, it flushes the bufio.Writer first
func (w Writer) Close() {
	f.writer.Flush()
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
