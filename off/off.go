// Package ljh provides classes that write OFF files
package off

import (
	"encoding/binary"
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

	file *os.File
}

type HeaderTDM struct {
	NumberOfRows    int
	NumberOfColumns int
	Row             int
	Column          int
}

type Header struct {
	Frameperiod   float64   `json:"frameperiod"`
	Format        string    `json:"File Format"`
	FormatVersion string    `json:"File Format Version"`
	TDM           HeaderTDM `json:"TDM"`
}

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
	_, err = w.file.Write(s)
	if err != nil {
		return err
	}
	_, err = w.file.WriteString("\n")
	if err != nil {
		return err
	}
	w.HeaderWritten = true
	return nil
}

func (w *Writer) WriteRecord(firstRisingSample int, rowcount int64, timestamp int64, data []float32) error {
	err := binary.Write(w.file, binary.LittleEndian, int32(len(data)))
	if err != nil {
		return err
	}
	err = binary.Write(w.file, binary.LittleEndian, int32(firstRisingSample))
	if err != nil {
		return err
	}
	err = binary.Write(w.file, binary.LittleEndian, rowcount)
	if err != nil {
		return err
	}
	err = binary.Write(w.file, binary.LittleEndian, timestamp)
	if err != nil {
		return err
	}
	err = binary.Write(w.file, binary.LittleEndian, data)
	if err != nil {
		return err
	}
	w.RecordsWritten += 1
	return nil
}

func (w Writer) Close() {
	w.file.Close()
}

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
	return nil
}
