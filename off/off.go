// Package off provides classes that write OFF files
// OFF files store TES pulses projected into a linear basis
// OFF files have a JSON header followed by a single newline
// after the header records are written sequentially in little endian format
// bytes		type			meaning
// 0-3      int32     recordSamples (could be calculated from nearest neighbor pulses in princple)
// 4-7      int32     recordPreSamples (could be calculated from nearest neighbor pulses in princple)
// 8-15     int64     framecount
// 16-23    int64     timestamp from time.Time.UnixNano()
// 24-27    float32   pretriggerMean (from raw data, not from modeled pulse, really shouldn't be neccesary, just in case for now!)
// 28-31    float32   residualStdDev (in raw data space, not Mahalanobis distance)
// 32-Z     float32   the NumberOfBases model coefficients of the pulse projected in to the model
// Z = 31+4*NumberOfBases
package off

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"internal/mysql"
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
	PixelInfo                 PixelInfo

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
	ReadoutInfo TimeDivisionMultiplexingInfo, pixelInfo PixelInfo) *Writer {
	writer := new(Writer)
	writer.ChannelIndex = ChannelIndex
	writer.ChannelName = ChannelName
	writer.ChannelNumberMatchingName = ChannelNumberMatchingName
	writer.MaxPresamples = MaxPresamples
	writer.MaxSamples = MaxSamples
	writer.FramePeriodSeconds = FramePeriodSeconds
	writer.FileFormat = "OFF"
	writer.FileFormatVersion = "0.3.0"
	writer.NumberOfBases, _ = Projectors.Dims()
	writer.ModelInfo = ModelInfo{Projectors: *NewArrayJsoner(Projectors), Basis: *NewArrayJsoner(Basis),
		Description: ModelDescription, projectors: Projectors, basis: Basis}
	writer.CreationInfo = CreationInfo{DastardVersion: DastardVersion, GitHash: GitHash,
		SourceName: SourceName, CreationTime: time.Now()}
	writer.ReadoutInfo = ReadoutInfo
	writer.PixelInfo = pixelInfo
	writer.fileName = fileName
	return writer
}

// ModelInfo stores info related to the model (aka basis, aka projectors) for printing to the file header, aids with json formatting
type ModelInfo struct {
	Projectors  ArrayJsoner
	projectors  *mat.Dense
	Basis       ArrayJsoner
	basis       *mat.Dense
	Description string
}

type PixelInfo struct {
	XPosition int
	YPosition int
	Name      string
}

// ArrayJsoner aids in formatting arrays for writing to JSON
type ArrayJsoner struct {
	Rows    int
	Cols    int
	SavedAs string
}

// NewArrayJsoner creates an ArrayJsoner from a mat.Dense
func NewArrayJsoner(array *mat.Dense) *ArrayJsoner {
	v := new(ArrayJsoner)
	v.Rows, v.Cols = array.Dims()
	v.SavedAs = "float64 binary data after header and before records. projectors first then basis, nbytes = rows*cols*8 for each projectors and basis"
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

// HeaderWritten returns true if header has been written.
func (w *Writer) HeaderWritten() bool {
	return w.headerWritten
}

// RecordsWritten return the nunber of records written.
func (w *Writer) RecordsWritten() int {
	return w.recordsWritten
}

// WriteHeader writes a header to the file
func (w *Writer) WriteHeader() error {
	if w.headerWritten {
		return errors.New("header already written")
	}
	s, err0 := json.MarshalIndent(w, "", "    ")
	if err0 != nil {
		return err0
	}
	if _, err := w.writer.Write(s); err != nil {
		return err
	}
	if _, err := w.writer.WriteString("\n"); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromSliceFloat64(w.ModelInfo.projectors.RawMatrix().Data)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromSliceFloat64(w.ModelInfo.basis.RawMatrix().Data)); err != nil {
		return err
	}
	w.headerWritten = true
	return nil
}

// WriteRecord writes a record to the file
func (w *Writer) WriteRecord(recordSamples int32, recordPreSamples int32, framecount int64,
	timestamp int64, pretriggerMean float32, pretriggerDelta float32, residualStdDev float32, data []float32) error {
	if len(data) != w.NumberOfBases {
		return fmt.Errorf("wrong number of bases, have %v, want %v", len(data), w.NumberOfBases)
	}
	if _, err := w.writer.Write(getbytes.FromInt32(int32(recordSamples))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt32(int32(recordPreSamples))); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt64(framecount)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromInt64(timestamp)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromFloat32(pretriggerMean)); err != nil {
		return err
	}
	if _, err := w.writer.Write(getbytes.FromFloat32(pretriggerDelta)); err != nil {
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
	if w.writer != nil {
		w.writer.Flush()
	}
}

// Close closes the file, it flushes the bufio.Writer first
func (w Writer) Close() {
	w.Flush()
	w.file.Close()
	msg := mysql.DatafileMessage{
		Fullpath:  w.fileName,
		Timestamp: time.Now(),
		Starting:  false,
		Records:   w.recordsWritten,
	}
	mysql.RecordDatafile(&msg)
}

// CreateFile creates a file at w.FileName
// must be called before WriteHeader or WriteRecord
func (w *Writer) CreateFile() error {
	if w.file != nil {
		return errors.New("file already exists")
	}
	file, err := os.Create(w.fileName)
	if err != nil {
		return err
	}
	w.file = file
	w.writer = bufio.NewWriterSize(w.file, 32768)
	msg := mysql.DatafileMessage{
		Fullpath:  w.fileName,
		Filetype:  "OFF",
		Timestamp: time.Now(),
		Starting:  true,
	}
	mysql.RecordDatafile(&msg)
	return nil
}
