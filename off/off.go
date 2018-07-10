// Package off provides classes that write OFF files
// OFF files store TES pulses projected into a linear basis
// OFF files have a JSON header followed by a single newline
// after the header records are written sequentially in little endian format
// bytes		type			meaning
// 0-3      int32     recordSamples (could be calculated from nearest neighbor pulses in princple)
// 4-7      int32     recordPreSamples (could be calculated from nearest neighbor pulses in princple)
// 8-15     int64     rowcount
// 16-23    int64     timestamp
// 24-27    float32   pretriggerMean (really shouldn't be neccesary, just in case for now!)
// 28-31    float32   residualStdDev (in raw data space, not Mahalanobis distance)
// 32-Z     float32   the NumberOfBases model coefficients of the pulse projected in to the model
// Z = 31+4*NumberOfBases
package off

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/usnistgov/dastard/getbytes"
	"gonum.org/v1/gonum/mat"
)

// Writer writes OFF files
type Writer struct {
	ChannelIndex              int
	ChannelName               string
	ChannelNumberMatchingName int
	MaxPresamples             int
	MaxSamples                int
	FramePeriodSeconds        float64
	FileFormat                string
	FileFormatVersion         string
	NumberOfBases             int
	ModelInfo                 ModelInfo
	CreationInfo              CreationInfo
	ReadoutInfo               TimeDivisionMultiplexingInfo

	// items not serialized to JSON header
	recordsWritten int
	fileName       string
	headerWritten  bool
	file           *os.File
	writer         *bufio.Writer
}

// NewWriter creates a new OFF writer. No file is created until the first call to WriteRecord
func NewWriter(fileName string, ChannelIndex int, ChannelName string, ChannelNumberMatchingName int,
	MaxPresamples int, MaxSamples int, FramePeriodSeconds float64,
	Projectors *mat.Dense, Basis *mat.Dense, ModelDescription string,
	DastardVersion string, GitHash string, SourceName string,
	ReadoutInfo TimeDivisionMultiplexingInfo) *Writer {
	writer := new(Writer)
	writer.ChannelIndex = ChannelIndex
	writer.ChannelName = ChannelName
	writer.ChannelNumberMatchingName = ChannelNumberMatchingName
	writer.MaxPresamples = MaxPresamples
	writer.FramePeriodSeconds = FramePeriodSeconds
	writer.NumberOfBases, _ = Projectors.Dims()
	writer.ModelInfo = ModelInfo{Projectors: *NewArrayJsoner(Projectors), Basis: *NewArrayJsoner(Basis),
		Description: ModelDescription}
	writer.CreationInfo = CreationInfo{DastardVersion: DastardVersion, GitHash: GitHash,
		SourceName: SourceName, CreationTime: time.Now()}
	writer.ReadoutInfo = ReadoutInfo
	writer.fileName = fileName
	return writer
}

// ModelInfo stores info related to the model (aka basis, aka projectors) for printing to the file header, aids with json formatting
type ModelInfo struct {
	Projectors  ArrayJsoner
	Basis       ArrayJsoner
	Description string
}

// ArrayJsoner aids in formatting arrays for writing to JSON
type ArrayJsoner struct {
	Float64ValuesBase64 string
	Rows                int
	Cols                int
}

// NewArrayJsoner creates an ArrayJsoner from a mat.Dense
func NewArrayJsoner(array *mat.Dense) *ArrayJsoner {
	v := new(ArrayJsoner)
	v.Rows, v.Cols = array.Dims()
	v.Float64ValuesBase64 = base64.StdEncoding.EncodeToString(getbytes.FromSliceFloat64(array.RawMatrix().Data))
	return v
}

// CreationInfo stores info related to file creation for printing to the file header, aids with json formatting
type CreationInfo struct {
	DastardVersion string
	GitHash        string
	SourceName     string
	CreationTime   time.Time
}

// TimeDivisionMultiplexingInfo stores info related to tdm readout for printing to the file header, aids with json formatting
type TimeDivisionMultiplexingInfo struct {
	NumberOfRows    int
	NumberOfColumns int
	NumberOfChans   int
	ColumnNum       int
	RowNum          int
}

// WriteHeader writes a header to the file
func (w *Writer) WriteHeader() error {
	if w.headerWritten {
		return errors.New("header already written")
	}
	s, err := json.MarshalIndent(w, "", "    ")
	if err != nil {
		return err
	}
	if _, err1 := w.writer.Write(s); err != nil {
		return err1
	}
	if _, err1 := w.writer.WriteString("\n"); err != nil {
		return err1
	}
	w.headerWritten = true
	return nil
}

// WriteRecord writes a record to the file
func (w *Writer) WriteRecord(recordSamples int32, recordPreSamples int32, rowcount int64,
	timestamp int64, pretriggerMean float32, residualStdDev float32, data []float32) error {
	if len(data) != w.NumberOfBases {
		return fmt.Errorf("wrong number of bases, have %v, want %v", len(data), w.NumberOfBases)
	}
	if _, err := w.writer.Write(getbytes.FromInt32(int32(recordSamples))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt32(int32(recordPreSamples))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt64(rowcount)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt64(timestamp)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromFloat32(pretriggerMean)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromFloat32(residualStdDev)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromSliceFloat32(data)); err != nil {
		return err
	}
	w.recordsWritten++
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
		file, err := os.Create(w.fileName)
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
