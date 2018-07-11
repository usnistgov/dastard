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
	PubRecordsChan    chan []*DataRecord
	PubSummariesChan  chan []*DataRecord
	LJH22             *ljh.Writer
	LJH3              *ljh.Writer3
	OFF               *off.Writer
	WritingPaused     bool
	numberWritten     int      // integrates up the total number written, reset any time writing starts or stops
	numberWrittenChan chan int // send the total number written to this channel after each write group
}

// SetPause changes the paused state to the given value of pause
func (dp *DataPublisher) SetPause(pause bool) {
	dp.WritingPaused = pause
	if dp.LJH22 != nil {
		dp.LJH22.Flush()
	}
	if dp.LJH3 != nil {
		dp.LJH3.Flush()
	}
	if dp.OFF != nil {
		dp.OFF.Flush()
	}
}

// SetOFF adds an OFF writer to dp, the .file attribute is nil, and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetOFF(ChannelIndex int, Presamples int, Samples int, FramesPerSample int,
	Timebase float64, TimestampOffset time.Time,
	NumberOfRows, NumberOfColumns, NumberOfChans, rowNum, colNum int,
	FileName, sourceName, chanName string, ChannelNumberMatchingName int,
	Projectors *mat.Dense, Basis *mat.Dense, ModelDescription string) {
	ReadoutInfo := off.TimeDivisionMultiplexingInfo{NumberOfRows: NumberOfRows,
		NumberOfColumns: NumberOfColumns,
		NumberOfChans:   NumberOfChans,
		ColumnNum:       colNum, RowNum: rowNum}
	w := off.NewWriter(FileName, ChannelIndex, chanName, ChannelNumberMatchingName, Presamples, Samples, Timebase,
		Projectors, Basis, ModelDescription, Build.Version, Build.Githash, sourceName, ReadoutInfo)
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
	FileName, sourceName, chanName string, ChannelNumberMatchingName int) {
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

// RemovePubRecords stops publing records on PortTrigs
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
	if dp.HasLJH22() && !dp.WritingPaused {
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
	if dp.HasLJH3() && !dp.WritingPaused {
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
	if dp.HasOFF() && !dp.WritingPaused {
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
				float32(record.pretrigMean), float32(record.residualStdDev), modelCoefs)
			if err != nil {
				return err
			}
		}
	}
	if (dp.HasLJH22() || dp.HasLJH3() || dp.HasOFF()) && !dp.WritingPaused {
		dp.numberWritten += len(records)
	}
	if dp.numberWrittenChan != nil {
		dp.numberWrittenChan <- dp.numberWritten
	}
	return nil
}

// messageSummaries makes a message with the following format for publishing on portTrigs
// Structure of the message header is defined in BINARY_FORMATS.md
// uint16: channel number
// uint8: header version number
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
	const headerVersion = uint8(0)

	header := new(bytes.Buffer)
	header.Write(getbytes.FromUint16(uint16(rec.channelIndex)))
	header.Write(getbytes.FromUint8(headerVersion))
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
	if rec.signed {
		dataType--
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

// PubRecordsChan is used to enable multiple different DataPublishers to publish on the same zmq pub socket
var PubRecordsChan chan []*DataRecord

// PubSummariesChan is used to enable multiple different DataPublishers to publish on the same zmq pub socket
var PubSummariesChan chan []*DataRecord

// configurePubRecordsSocket should be run exactly one time
// it initializes PubFeederChan and launches a goroutine (with no way to stop it at the moment)
// that reads from PubFeederChan and publishes records on a ZMQ PUB socket at port PortTrigs
// this way even if go routines in different threads want to publish records, they all use the same
// zmq port.
func configurePubRecordsSocket() (err error) {
	if PubRecordsChan != nil {
		return fmt.Errorf("run configurePubRecordsSocket only one time")
	}
	PubRecordsChan, err = startSocket(Ports.Trigs, messageRecords)
	return
}

// configurePubSummariesSocket should be run exactly one time; analogue of configurePubRecordsSocket
func configurePubSummariesSocket() (err error) {
	if PubSummariesChan != nil {
		return fmt.Errorf("run configurePubSummariesSocket only one time")
	}
	PubSummariesChan, err = startSocket(Ports.Summaries, messageSummaries)
	return
}

// startSocket sets up a ZMQ publisher socket and starts a goroutine to publish
// messages based on any records that appear on a new channel. Returns the
// channel for other routines to fill.
// *** This looks like it could be replaced by PubChanneler, but tests show terrible performance with Channeler ***
func startSocket(port int, converter func(*DataRecord) [][]byte) (chan []*DataRecord, error) {
	const publishChannelDepth = 500
	// The following could could be a PubChanneler
	pubchan := make(chan []*DataRecord, publishChannelDepth)
	hostname := fmt.Sprintf("tcp://*:%d", port)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return nil, err
	}
	go func() {
		defer pubSocket.Destroy()
		for {
			records := <-pubchan
			for _, record := range records {
				message := converter(record)
				err := pubSocket.SendMessage(message)
				if err != nil {
					panic("zmq send error")
				}
			}
		}
	}()
	return pubchan, nil
}

// rawTypeToBytes convert a []RawType to []byte using unsafe
// see https://stackoverflow.com/questions/11924196/convert-between-slices-of-different-types?utm_medium=organic&utm_source=google_rich_qa&utm_campaign=google_rich_qa
func rawTypeToBytes(d []RawType) []byte {
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&d))
	header.Cap *= 2 // byte takes up half the space of RawType
	header.Len *= 2
	data := *(*[]byte)(unsafe.Pointer(&header))
	return data
}

// rawTypeToUint16convert a []RawType to []uint16 using unsafe
// see https://stackoverflow.com/questions/11924196/convert-between-slices-of-different-types?utm_medium=organic&utm_source=google_rich_qa&utm_campaign=google_rich_qa
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

// PublishSync is used to synchronize the publication of number of records written
type PublishSync struct {
	numberWrittenChans []chan int
	NumberWritten      []int
	abort              chan struct{} // This can signal the Run() goroutine to stop
}

// NewPublishSync returns a *PublishSync for nchan channels
func NewPublishSync(nchan int) *PublishSync {
	numberWrittenChans := make([]chan int, nchan)
	for i := 0; i < nchan; i++ {
		numberWrittenChans[i] = make(chan int)
	}
	return &PublishSync{numberWrittenChans: numberWrittenChans, abort: make(chan struct{}),
		NumberWritten: make([]int, nchan)}
}

// Run runs, collects number of records published, publishes summaries
// should be called as a goroutine
func (ps *PublishSync) Run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		default:
			// get data from all PrimaryTrigs channels
			for i := 0; i < len(ps.numberWrittenChans); i++ {
				select {
				case <-ps.abort:
					return
				case n := <-ps.numberWrittenChans[i]:
					ps.NumberWritten[i] = n
				}
			}
		case <-ticker.C:
			clientMessageChan <- ClientUpdate{tag: "NUMBERWRITTEN",
				state: ps} // only exported fields are serialized
		}
	}
}

// Stop causes the Run() goroutine to end at the next appropriate moment.
func (ps *PublishSync) Stop() {
	close(ps.abort)
}
