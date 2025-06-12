package dastard

import (
	"bytes"
	"fmt"
	"time"

	"github.com/usnistgov/dastard/internal/dastarddb"
	"github.com/usnistgov/dastard/internal/getbytes"
	"github.com/usnistgov/dastard/internal/ljh"
	"github.com/usnistgov/dastard/internal/off"
	"gonum.org/v1/gonum/mat"

	"github.com/pebbe/zmq4"
)

// DataPublisher contains many optional methods for publishing data; any methods that are non-nil
// will be used in each call to PublishData.
type DataPublisher struct {
	PubRecordsChan   chan []*DataRecord
	PubSummariesChan chan []*DataRecord
	LJH22            *ljh.Writer
	LJH3             *ljh.Writer3
	OFF              *off.Writer
	WritingPaused    bool
	numberWritten    int // integrates up the total number written; reset any time writing starts or stops
}

// SetPause changes the paused state to the value of `pause`
func (dp *DataPublisher) SetPause(pause bool) {
	dp.WritingPaused = pause
	dp.Flush()
}

// Flush calls Flush for each active writer that has a Flush command (LJH22, LJH3, OFF)
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
	NumberOfRows, NumberOfColumns, NumberOfChans, SubframeDivisions, rowNum, colNum, SubframeOffset int,
	FileName, sourceName, chanName string, ChannelNumberMatchingName int,
	Projectors *mat.Dense, Basis *mat.Dense, ModelDescription string, pixel Pixel) {
	ReadoutInfo := off.TimeDivisionMultiplexingInfo{
		NumberOfRows:      NumberOfRows,
		NumberOfColumns:   NumberOfColumns,
		NumberOfChans:     NumberOfChans,
		SubframeDivisions: SubframeDivisions,
		ColumnNum:         colNum,
		RowNum:            rowNum,
		SubframeOffset:    SubframeOffset}
	PixelInfo := off.PixelInfo{XPosition: pixel.X, YPosition: pixel.Y, Name: pixel.Name}
	w := off.NewWriter(FileName, ChannelIndex, chanName, ChannelNumberMatchingName, Presamples, Samples, Timebase,
		Projectors, Basis, ModelDescription, Build.Version, Build.Githash, sourceName, ReadoutInfo, PixelInfo)
	dp.OFF = w
	dp.numberWritten = 0
}

// HasOFF returns true if OFF is non-nil; used to decide if writing to OFF should occur
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
	NumberOfRows, NumberOfColumns, SubframeDivisions, SubframeOffset int,
	FileName string) {
	w := ljh.Writer3{
		ChannelIndex:      ChannelIndex,
		Timebase:          Timebase,
		NumberOfRows:      NumberOfRows,
		NumberOfColumns:   NumberOfColumns,
		SubframeDivisions: SubframeDivisions,
		SubframeOffset:    SubframeOffset,
		FileName:          FileName,
	}
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

// SetLJH22 adds an LJH22 writer to dp.
// The Writer.file attribute is nil and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetLJH22(ChannelIndex int, Presamples int, Samples int,
	FramesPerSample int, Timebase float64, TimestampOffset time.Time,
	NumberOfRows, NumberOfColumns, NumberOfChans, SubframeDivisions, rowNum, colNum, SubframeOffset int,
	FileName, sourceName, chanName string, ChannelNumberMatchingName int, pixel Pixel,
	filemsg *dastarddb.FileMessage) {
	w := ljh.Writer{
		ChannelIndex:              ChannelIndex,
		Presamples:                Presamples,
		Samples:                   Samples,
		FramesPerSample:           FramesPerSample,
		Timebase:                  Timebase,
		TimestampOffset:           TimestampOffset,
		NumberOfRows:              NumberOfRows,
		NumberOfColumns:           NumberOfColumns,
		NumberOfChans:             NumberOfChans,
		SubframeDivisions:         SubframeDivisions,
		FileName:                  FileName,
		DastardVersion:            Build.Version,
		GitHash:                   Build.Githash,
		ChanName:                  chanName,
		ChannelNumberMatchingName: ChannelNumberMatchingName,
		SourceName:                sourceName,
		ColumnNum:                 colNum,
		RowNum:                    rowNum,
		SubframeOffset:            SubframeOffset,
		PixelXPosition:            pixel.X,
		PixelYPosition:            pixel.Y,
		PixelName:                 pixel.Name,
		FileMessage:               filemsg,
		DB:                        DB,
	}
	dp.LJH22 = &w
	dp.WritingPaused = false
	dp.numberWritten = 0
}

// HasLJH22 returns true if LJH22 is non-nil; used to decide if writing to LJH22 should occur
func (dp *DataPublisher) HasLJH22() bool {
	return dp.LJH22 != nil
}

// RemoveLJH22 closes existing LJH22 file and assigns .LJH22=nil
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

// SetPubRecords starts publishing records with zmq4 over tcp at port=PortTrigs
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

// SetPubSummaries starts publishing records with zmq4 over tcp at port=PortSummaries
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

// PublishData publishes each record in the slice `records` onto each
// active specific publisher, including the ZMQ record port, the ZMQ
// summary port, and the 3 possible disk file types (LJH22, LJH3, OFF).
// The first step is to publish the full record and/or a record summary to the relevant ZMQ ports.
// The second is to store the records into any active LJH22, LJH3, and/or OFF writers.
func (dp *DataPublisher) PublishData(records []*DataRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Publish the records to the ZMQ ports for full records and/or record summaries.
	if dp.HasPubRecords() {
		dp.PubRecordsChan <- records
	}
	if dp.HasPubSummaries() {
		dp.PubSummariesChan <- records
	}

	// If writing is paused or there are no active file outputs, then we are done publishing.
	if dp.WritingPaused {
		return nil
	}
	if !(dp.HasLJH22() || dp.HasLJH3() || dp.HasOFF()) {
		return nil
	}

	// If we get here, there is one or more output active.
	// The LJH and OFF files are _not_ created until they are needed, so each type first checks if the file
	// is already opened and has a header written.
	if dp.HasLJH22() {
		if !dp.LJH22.HeaderWritten {
			err := dp.LJH22.CreateFile()
			if err != nil {
				return err
			}
			t0 := records[0].trigTime
			dp.LJH22.WriteHeader(t0)
		}
		for _, record := range records {
			nano := record.trigTime.UnixNano()
			dp.LJH22.WriteRecord(int64(record.trigFrame), int64(nano)/1000, rawTypeToUint16(record.data))
		}
	}
	if dp.HasLJH3() {
		if !dp.LJH3.HeaderWritten {
			err := dp.LJH3.CreateFile()
			if err != nil {
				return err
			}
			dp.LJH3.WriteHeader()
		}
		for _, record := range records {
			nano := record.trigTime.UnixNano()
			dp.LJH3.WriteRecord(int32(record.presamples+1), int64(record.trigFrame), int64(nano)/1000, rawTypeToUint16(record.data))
		}
	}
	if dp.HasOFF() {
		if !dp.OFF.HeaderWritten() {
			err := dp.OFF.CreateFile()
			if err != nil {
				return err
			}
			dp.OFF.WriteHeader()
		}
		for _, record := range records {
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
//
//	end of first message packet
//	modelCoefs, each coef is float32, length can vary
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
	// DataSegment.signed is set deep within a source,
	// then dsp.signed is set equal to DataSegment.signed in process data,
	// then DataRecord.signed is set equal to dsp.signed when record is generated.
	if rec.signed {
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
// It initializes PubRecordsChan and launches a goroutine
// that reads from PubRecordsChan and publishes records on a ZMQ PUB socket at port PortTrigs.
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
func startSocket(port int, converter func(*DataRecord) [][]byte) (chan []*DataRecord, error) {

	// Not totally sure how to choose this, but it should probably be
	// at least as large as number of channels.
	const publishChannelDepth = 500
	pubchan := make(chan []*DataRecord, publishChannelDepth)

	//  Socket to talk to clients
	hostname := fmt.Sprintf("tcp://*:%d", port)
	pubSocket, err := zmq4.NewSocket(zmq4.PUB)
	if err != nil {
		return nil, err
	}
	pubSocket.SetSndhwm(100)
	if err = pubSocket.Bind(hostname); err != nil {
		return nil, err
	}

	go func() {
		for {
			records, ok := <-pubchan
			if !ok { // Destroy socket when pubchan is closed and drained
				pubSocket.Close()
				return
			}
			for _, record := range records {
				message := converter(record)
				if _, err := pubSocket.SendMessage(message); err != nil {
					ProblemLogger.Println("zmq send error publishing a triggered record:", err)
				}
			}
		}
	}()
	return pubchan, nil
}
