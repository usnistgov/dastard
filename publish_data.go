package dastard

import (
	"bytes"
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/usnistgov/dastard/getbytes"
	"github.com/usnistgov/dastard/ljh"
	"github.com/usnistgov/dastard/off"
	"gonum.org/v1/gonum/mat"

	czmq "github.com/zeromq/goczmq"
)

// DataPublisher contains many optional methods for publishing data, any methods that are non-nil will be used
// in each call to PublishData
type DataPublisher struct {
	PubRecordsChan   chan []*DataRecord
	PubSummariesChan chan []*DataRecord
	LJH22            *ljh.Writer
	LJH3             *ljh.Writer3
	OFF              *off.Writer
	WritingPaused    bool
	numberWritten    int // integrates up the total number written, reset any time writing starts or stops
}

// SetPause changes the paused state to the given value of pause
func (dp *DataPublisher) SetPause(pause bool) {
	dp.WritingPaused = pause
	dp.Flush()
}

// Flush calls Flush for each writer that has a Flush command (LJH22, LJH3, OFF)
func (dp *DataPublisher) Flush() {
	if dp.HasLJH22() {
		dp.LJH22.Flush()
	}
	if dp.HasLJH3() {
		dp.LJH3.Flush()
	}
	if dp.HasOFF() {
		dp.OFF.Flush()
	}
}

// SetOFF adds an OFF writer to dp, the .file attribute is nil, and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetOFF(ChannelIndex int, Presamples int, Samples int, FramesPerSample int,
	Timebase float64, TimestampOffset time.Time,
	NumberOfRows, NumberOfColumns, NumberOfChans, rowNum, colNum int,
	FileName, sourceName, chanName string, ChannelNumberMatchingName int,
	Projectors *mat.Dense, Basis *mat.Dense, ModelDescription string, pixel Pixel) {
	ReadoutInfo := off.TimeDivisionMultiplexingInfo{NumberOfRows: NumberOfRows,
		NumberOfColumns: NumberOfColumns,
		NumberOfChans:   NumberOfChans,
		ColumnNum:       colNum, RowNum: rowNum}
	PixelInfo := off.PixelInfo{XPosition: pixel.X, YPosition: pixel.Y, Name: pixel.Name}
	w := off.NewWriter(FileName, ChannelIndex, chanName, ChannelNumberMatchingName, Presamples, Samples, Timebase,
		Projectors, Basis, ModelDescription, Build.Version, Build.Githash, sourceName, ReadoutInfo, PixelInfo)
	dp.OFF = w
	dp.numberWritten = 0
}

// HasOFF returns true if OFF is non-nil, eg if writing to OFF is occuring
func (dp *DataPublisher) HasOFF() bool {
	return dp.OFF != nil
}

// RemoveOFF closes any existing OFF file and assign .OFF=nil
func (dp *DataPublisher) RemoveOFF() {
	if dp.OFF != nil {
		dp.OFF.Close()
	}
	dp.OFF = nil
	dp.numberWritten = 0

}

// SetLJH3 adds an LJH3 writer to dp, the .file attribute is nil, and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetLJH3(ChannelIndex int, Timebase float64,
	NumberOfRows int, NumberOfColumns int, FileName string) {
	w := ljh.Writer3{ChannelIndex: ChannelIndex,
		Timebase:        Timebase,
		NumberOfRows:    NumberOfRows,
		NumberOfColumns: NumberOfColumns,
		FileName:        FileName}
	dp.LJH3 = &w
	dp.WritingPaused = false
	dp.numberWritten = 0
}

// HasLJH3 returns true if LJH3 is non-nil, eg if writing to LJH3 is occuring
func (dp *DataPublisher) HasLJH3() bool {
	return dp.LJH3 != nil
}

// RemoveLJH3 closes existing LJH3 file and assign .LJH3=nil
func (dp *DataPublisher) RemoveLJH3() {
	if dp.LJH3 != nil {
		dp.LJH3.Close()
	}
	dp.LJH3 = nil
	dp.numberWritten = 0
}

// SetLJH22 adds an LJH22 writer to dp, the .file attribute is nil, and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetLJH22(ChannelIndex int, Presamples int, Samples int, FramesPerSample int,
	Timebase float64, TimestampOffset time.Time,
	NumberOfRows, NumberOfColumns, NumberOfChans, rowNum, colNum int,
	FileName, sourceName, chanName string, ChannelNumberMatchingName int, pixel Pixel) {
	w := ljh.Writer{ChannelIndex: ChannelIndex,
		Presamples:                Presamples,
		Samples:                   Samples,
		FramesPerSample:           FramesPerSample,
		Timebase:                  Timebase,
		TimestampOffset:           TimestampOffset,
		NumberOfRows:              NumberOfRows,
		NumberOfColumns:           NumberOfColumns,
		NumberOfChans:             NumberOfChans,
		FileName:                  FileName,
		DastardVersion:            Build.Version,
		GitHash:                   Build.Githash,
		ChanName:                  chanName,
		ChannelNumberMatchingName: ChannelNumberMatchingName,
		SourceName:                sourceName,
		ColumnNum:                 colNum,
		RowNum:                    rowNum,
		PixelXPosition:            pixel.X,
		PixelYPosition:            pixel.Y,
		PixelName:                 pixel.Name,
	}
	dp.LJH22 = &w
	dp.WritingPaused = false
	dp.numberWritten = 0
}

// HasLJH22 returns true if LJH22 is non-nil, used to decide if writeint to LJH22 should occur
func (dp *DataPublisher) HasLJH22() bool {
	return dp.LJH22 != nil
}

// RemoveLJH22 closes existing LJH22 file and assign .LJH22=nil
func (dp *DataPublisher) RemoveLJH22() {
	if dp.LJH22 != nil {
		dp.LJH22.Close()
	}
	dp.LJH22 = nil
	dp.numberWritten = 0
}

// HasPubRecords return true if publishing records on PortTrigs Pub is occuring
func (dp *DataPublisher) HasPubRecords() bool {
	return dp.PubRecordsChan != nil
}

// SetPubRecords starts publishing records with czmq over tcp at port=PortTrigs
func (dp *DataPublisher) SetPubRecords() {
	if PubRecordsChan == nil {
		configurePubRecordsSocket()
	}
	if dp.PubRecordsChan == nil {
		dp.PubRecordsChan = PubRecordsChan
	}
}

// RemovePubRecords stops publishing records on PortTrigs
func (dp *DataPublisher) RemovePubRecords() {
	dp.PubRecordsChan = nil
}

// HasPubSummaries return true if publishing summaries on PortSummaries Pub is occuring
func (dp *DataPublisher) HasPubSummaries() bool {
	return dp.PubSummariesChan != nil
}

// SetPubSummaries starts publishing records with czmq over tcp at port=PortSummaries
func (dp *DataPublisher) SetPubSummaries() {
	if PubSummariesChan == nil {
		configurePubSummariesSocket()
	}
	if dp.PubSummariesChan == nil {
		dp.PubSummariesChan = PubSummariesChan
	}
}

// RemovePubSummaries stop publing summaries on PortSummaires
func (dp *DataPublisher) RemovePubSummaries() {
	dp.PubSummariesChan = nil
}

// PublishData looks at each member of DataPublisher, and if it is non-nil, publishes each record into that member
func (dp *DataPublisher) PublishData(records []*DataRecord) error {
	if dp.HasPubRecords() {
		dp.PubRecordsChan <- records
	}
	if dp.HasPubSummaries() {
		dp.PubSummariesChan <- records
	}
	if dp.WritingPaused {
		return nil
	}
	if !(dp.HasLJH22() || dp.HasLJH3() || dp.HasOFF()) {
		return nil
	}
	if dp.HasLJH22() {
		for _, record := range records {
			if !dp.LJH22.HeaderWritten { // MATTER doesn't create ljh files until at least one record exists, let us do the same
				// if the file doesn't exists yet, create it and write header
				err := dp.LJH22.CreateFile()
				if err != nil {
					return err
				}
				dp.LJH22.WriteHeader(record.trigTime)
			}
			nano := record.trigTime.UnixNano()
			dp.LJH22.WriteRecord(int64(record.trigFrame), int64(nano)/1000, rawTypeToUint16(record.data))
		}
	}
	if dp.HasLJH3() {
		for _, record := range records {
			if !dp.LJH3.HeaderWritten { // MATTER doesn't create ljh files until at least one record exists, let us do the same
				// if the file doesn't exists yet, create it and write header
				err := dp.LJH3.CreateFile()
				if err != nil {
					return err
				}
				dp.LJH3.WriteHeader()
			}
			nano := record.trigTime.UnixNano()
			dp.LJH3.WriteRecord(int32(record.presamples+1), int64(record.trigFrame), int64(nano)/1000, rawTypeToUint16(record.data))
		}
	}
	if dp.HasOFF() {
		for _, record := range records {
			if !dp.OFF.HeaderWritten() { // MATTER doesn't create ljh files until at least one record exists, let us do the same
				// if the file doesn't exists yet, create it and write header
				err := dp.OFF.CreateFile()
				if err != nil {
					return err
				}
				dp.OFF.WriteHeader()
			}
			modelCoefs := make([]float32, len(record.modelCoefs))
			for i, v := range record.modelCoefs {
				modelCoefs[i] = float32(v)
			}
			err := dp.OFF.WriteRecord(int32(len(record.data)), int32(record.presamples), int64(record.trigFrame), record.trigTime.UnixNano(),
				float32(record.pretrigMean), float32(record.pretrigDelta), float32(record.residualStdDev), modelCoefs)
			if err != nil {
				return err
			}
		}
	}
	dp.numberWritten += len(records)
	return nil
}

// messageSummaries makes a message with the following format for publishing on portTrigs
// Structure of the message header is defined in BINARY_FORMATS.md
// uint16: channel number
// uint16: header version number
// uint32: bits: Presamples
// uint32: length of record
// float32: pretrigMean
// float32: peakValue
// float32: pulseRMS
// float32: pulseAverage
// float32: residualStdDev
// uint64: UnixNano trigTime
// uint64: trigFrame
//  end of first message packet
//  modelCoefs, each coef is float32, length can vary
func messageSummaries(rec *DataRecord) [][]byte {
	const headerVersion = uint16(0)

	header := new(bytes.Buffer)
	header.Write(getbytes.FromUint16(uint16(rec.channelIndex)))
	header.Write(getbytes.FromUint16(headerVersion))
	header.Write(getbytes.FromUint32(uint32(rec.presamples)))
	header.Write(getbytes.FromUint32(uint32(len(rec.data))))
	header.Write(getbytes.FromFloat32(float32(rec.pretrigMean)))
	header.Write(getbytes.FromFloat32(float32(rec.peakValue)))
	header.Write(getbytes.FromFloat32(float32(rec.pulseRMS)))
	header.Write(getbytes.FromFloat32(float32(rec.pulseAverage)))
	header.Write(getbytes.FromFloat32(float32(rec.residualStdDev)))
	nano := rec.trigTime.UnixNano()
	header.Write(getbytes.FromInt64(nano))
	header.Write(getbytes.FromInt64(int64(rec.trigFrame)))

	return [][]byte{header.Bytes(), getbytes.FromSliceFloat64(rec.modelCoefs)}
}

// messageRecords makes a message with the following format for publishing on portTrigs
// Structure of the message header is defined in BINARY_FORMATS.md
// uint16: channel number
// uint8: header version number
// uint8: code for data type (0-1 = 8 bits; 2-3 = 16; 4-5 = 32; 6-7 = 64; odd=uint; even=int)
// uint32: # of pre-trigger samples
// uint32: # of samples, total
// float32: sample period, in seconds (float)
// float32: volts per arb conversion (float)
// uint64: trigger time, in ns since epoch 1970
// uint64: trigger frame #
// end of first message packet
// data, each sample is uint16, length given above
func messageRecords(rec *DataRecord) [][]byte {

	const headerVersion = uint8(0)
	dataType := uint8(3)
	if rec.signed { // DataSegment.signed is set deep within a source, then dsp.signed is set equal to DataSegment.signed in process data, then DataRecord.signed is set equal to dsp.signed upon record generation
		dataType = uint8(2)
	}
	header := new(bytes.Buffer)
	header.Write(getbytes.FromUint16(uint16(rec.channelIndex)))
	header.Write(getbytes.FromUint8(headerVersion))
	header.Write(getbytes.FromUint8(dataType))
	header.Write(getbytes.FromUint32(uint32(rec.presamples)))
	header.Write(getbytes.FromUint32(uint32(len(rec.data))))
	header.Write(getbytes.FromFloat32(rec.sampPeriod))
	header.Write(getbytes.FromFloat32(rec.voltsPerArb))
	nano := rec.trigTime.UnixNano()
	header.Write(getbytes.FromInt64(nano))
	header.Write(getbytes.FromUint64(uint64(rec.trigFrame)))

	data := rawTypeToBytes(rec.data)
	return [][]byte{header.Bytes(), data}
}

// Two library-global variables to allow sharing of zmq publisher sockets:

// PubRecordsChan is used to enable multiple different DataPublishers to publish on the same zmq pub socket
var PubRecordsChan chan []*DataRecord

// PubSummariesChan is used to enable multiple different DataPublishers to publish on the same zmq pub socket
var PubSummariesChan chan []*DataRecord

// configurePubRecordsSocket should be run exactly one time.
// It initializes PubFeederChan and launches a goroutine
// that reads from PubFeederChan and publishes records on a ZMQ PUB socket at port PortTrigs.
// This way even if goroutines in different threads want to publish records, they all use the same
// zmq port. The goroutine can be stopped by closing PubRecordsChan.
func configurePubRecordsSocket() error {
	if PubRecordsChan != nil {
		return fmt.Errorf("run configurePubRecordsSocket only one time")
	}
	var err error
	PubRecordsChan, err = startSocket(Ports.Trigs, messageRecords)
	return err
}

// configurePubSummariesSocket should be run exactly one time; analogue of configurePubRecordsSocket
func configurePubSummariesSocket() error {
	if PubSummariesChan != nil {
		return fmt.Errorf("run configurePubSummariesSocket only one time")
	}
	var err error
	PubSummariesChan, err = startSocket(Ports.Summaries, messageSummaries)
	return err
}

// startSocket sets up a ZMQ publisher socket and starts a goroutine to publish
// messages based on any records that appear on a new channel. Returns the
// channel for other routines to fill. Close that channel to destroy the socket.
//
// *** This looks like it could be replaced by PubChanneler, but tests showed terrible
// performance with Channeler ***
func startSocket(port int, converter func(*DataRecord) [][]byte) (chan []*DataRecord, error) {
	const publishChannelDepth = 500 // not totally sure how to choose this, but it should probably be
	// at least as large as number of channels
	pubchan := make(chan []*DataRecord, publishChannelDepth)
	hostname := fmt.Sprintf("tcp://*:%d", port)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return nil, err
	}
	pubSocket.SetSndhwm(10)
	go func() {
		for {
			records, ok := <-pubchan
			if !ok { // Destroy socket when pubchan is closed and drained
				pubSocket.Destroy()
				return
			}
			for _, record := range records {
				message := converter(record)
				err := pubSocket.SendMessage(message)
				if err != nil {
					ProblemLogger.Println("zmq send error publishing a triggered record:", err)
				}
			}
		}
	}()
	return pubchan, nil
}

// rawTypeToBytes convert a []RawType to []byte using unsafe
// see https://stackoverflow.com/questions/11924196/convert-between-slices-of-different-types
func rawTypeToBytes(d []RawType) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 2 // byte takes up half the space of RawType
	header.Len *= 2
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// rawTypeToUint16convert a []RawType to []uint16 using unsafe
func rawTypeToUint16(d []RawType) []uint16 {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	data := *(*[]uint16)(unsafe.Pointer(&header))
	return data
}

func bytesToRawType(b []byte) []RawType {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&b))
	const ratio = int(unsafe.Sizeof(RawType(0)))
	header.Cap /= ratio // byte takes up twice the space of RawType
	header.Len /= ratio
	data := *(*[]RawType)(unsafe.Pointer(&header))
	return data
}

// bytesToInt32 converts a []byte slice to an []int32 slice, which is
// how we interpret the raw Abaco data.
func bytesToInt32(b []byte) []int32 {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&b))
	const ratio = int(unsafe.Sizeof(int32(0)))
	header.Cap /= ratio // byte takes up ratio times the space of RawType
	header.Len /= ratio
	data := *(*[]int32)(unsafe.Pointer(&header))
	return data
}
