package dastard

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/usnistgov/dastard/ljh"
	"github.com/usnistgov/dastard/off"

	czmq "github.com/zeromq/goczmq"
)

// DataPublisher contains many optional methods for publishing data, any methods that are non-nil will be used
// in each call to PublishData
type DataPublisher struct {
	PubFeederChan chan []*DataRecord
	LJH22         *ljh.Writer
	LJH3          *ljh.Writer3
	OFF           *off.Writer
}

// SetOFF adds an OFF writer to dp, the .file attribute is nil, and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetOFF(ChanNum int, Timebase float64,
	NumberOfRows int, NumberOfColumns int, NumberOfBases int, FileName string) {
	w := off.Writer{ChanNum: ChanNum,
		Timebase:        Timebase,
		NumberOfRows:    NumberOfRows,
		NumberOfColumns: NumberOfColumns,
		FileName:        FileName,
		NumberOfBases:   NumberOfBases, ProjectorsPlaceHolder: "projectors go here",
		BasisPlaceHolder: "basis goes here", NoiseWhitenerPlaceHolder: "noise whitener goes here"}
	dp.OFF = &w
}

// HasLJH22 returns true if LJH22 is non-nil, used to decide if writeint to LJH22 should occur
func (dp *DataPublisher) HasOFF() bool {
	return dp.OFF != nil
}
func (dp *DataPublisher) RemoveOFF() {
	dp.OFF.Close()
	dp.OFF = nil
}

// SetLJH3 adds an LJH3 writer to dp, the .file attribute is nil, and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetLJH3(ChanNum int, Timebase float64,
	NumberOfRows int, NumberOfColumns int, FileName string) {
	w := ljh.Writer3{ChanNum: ChanNum,
		Timebase:        Timebase,
		NumberOfRows:    NumberOfRows,
		NumberOfColumns: NumberOfColumns,
		FileName:        FileName}
	dp.LJH3 = &w
}

// HasLJH22 returns true if LJH22 is non-nil, used to decide if writeint to LJH22 should occur
func (dp *DataPublisher) HasLJH3() bool {
	return dp.LJH3 != nil
}
func (dp *DataPublisher) RemoveLJH3() {
	dp.LJH3.Close()
	dp.LJH3 = nil
}

// SetLJH22 adds an LJH22 writer to dp, the .file attribute is nil, and will be instantiated upon next call to dp.WriteRecord
func (dp *DataPublisher) SetLJH22(ChanNum int, Presamples int, Samples int, Timebase float64, TimestampOffset float64,
	NumberOfRows int, NumberOfColumns int, FileName string) {
	w := ljh.Writer{ChanNum: ChanNum,
		Presamples:      Presamples,
		Samples:         Samples,
		Timebase:        Timebase,
		TimestampOffset: TimestampOffset,
		NumberOfRows:    NumberOfRows,
		NumberOfColumns: NumberOfColumns,
		FileName:        FileName}
	dp.LJH22 = &w
}

// HasLJH22 returns true if LJH22 is non-nil, used to decide if writeint to LJH22 should occur
func (dp *DataPublisher) HasLJH22() bool {
	return dp.LJH22 != nil
}
func (dp *DataPublisher) RemoveLJH22() {
	dp.LJH22.Close()
	dp.LJH22 = nil
}
func (dp *DataPublisher) HasPubFeederChan() bool {
	return dp.PubFeederChan != nil
}

func (dp *DataPublisher) SetPubFeederChan() {
	if PubFeederChan == nil {
		panic("run configurePubSocket before SetPubFeederChan")
	}
	dp.PubFeederChan = PubFeederChan
}
func (dp *DataPublisher) RemovePubFeederChan() {
	dp.PubFeederChan = nil
}

var PubFeederChan chan []*DataRecord

// configurePubSocket should be run exactly one time
// it initializes PubFeederChan and launches a goroutine (with no way to stop it at the moment)
// that reads from PubFeederChan and publishes records on a ZMQ PUB socket at port PortTrigs
// this way even if go routines in different threads want to publish records, they all use the same
// zmq port
// *** This can probably be replaced by czmq.PubChanneler ***
func configurePubSocket() error {
	if PubFeederChan != nil {
		panic("run configurePubSocket only one time")
	}
	const publishChannelDepth = 500
	// I think this could be a PubChanneler
	PubFeederChan = make(chan []*DataRecord, publishChannelDepth)
	hostname := fmt.Sprintf("tcp://*:%d", PortTrigs)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return err
	}
	go func() {
		defer pubSocket.Destroy()
		for {
			records := <-PubFeederChan
			for _, record := range records {
				header, signal := packet(record)
				err := pubSocket.SendFrame(header, czmq.FlagMore)
				if err != nil {
					panic("zmq send error")
				}
				err = pubSocket.SendFrame(signal, czmq.FlagNone)
				if err != nil {
					panic("zmq send error")
				}
			}
		}

	}()
	return nil
}

// PublishData looks at each member of DataPublisher, and if it is non-nil, publishes each record into that member
func (dp DataPublisher) PublishData(records []*DataRecord) error {
	if dp.HasPubFeederChan() {
		dp.PubFeederChan <- records
	}
	if dp.HasLJH22() {
		for _, record := range records {
			if !dp.LJH22.HeaderWritten { // MATTER doesn't create ljh files until at least one record exists, let us do the same
				// if the file doesn't exists yet, create it and write header
				err := dp.LJH22.CreateFile()
				if err != nil {
					return err
				}
				dp.LJH22.WriteHeader()
			}
			nano := record.trigTime.UnixNano()
			data := make([]uint16, len(record.data))
			for i, v := range record.data {
				data[i] = uint16(v)
			}
			dp.LJH22.WriteRecord(int64(record.trigFrame), int64(nano)/1000, data)
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
			data := make([]uint16, len(record.data))
			for i, v := range record.data {
				data[i] = uint16(v)
			}
			dp.LJH3.WriteRecord(record.presamples+1, int64(record.trigFrame), int64(nano)/1000, data)
		}
	}
	if dp.HasOFF() {
		for _, record := range records {
			if !dp.OFF.HeaderWritten { // MATTER doesn't create ljh files until at least one record exists, let us do the same
				// if the file doesn't exists yet, create it and write header
				err := dp.OFF.CreateFile()
				if err != nil {
					return err
				}
				dp.OFF.WriteHeader()
			}
			nano := record.trigTime.UnixNano()
			data := make([]uint16, len(record.data))
			for i, v := range record.data {
				data[i] = uint16(v)
			}
			modelCoefs := make([]float32, len(record.modelCoefs))
			for i, v := range record.modelCoefs {
				modelCoefs[i] = float32(v)
			}
			dp.OFF.WriteRecord(record.presamples+1, int64(record.trigFrame), int64(nano)/1000, modelCoefs)
		}
	}
	return nil
}

func packet(rec *DataRecord) ([]byte, []byte) {
	// Structure of the message header is defined in BINARY_FORMATS.md
	// 16 bits: channel number
	//  8 bits: header version number
	//  8 bits: code for data type (0-1 = 8 bits; 2-3 = 16; 4-5 = 32; 6-7 = 64; odd=uint; even=int)
	// 32 bits: # of pre-trigger samples
	// 32 bits: # of samples, total
	// 32 bits: sample period, in seconds (float)
	// 32 bits: volts per arb conversion (float)
	// 64 bits: trigger time, in ns since epoch 1970
	// 64 bits: trigger frame #

	const headerVersion = uint8(0)
	const dataType = uint8(3) // will want to assert about RawType somehow?

	// header := make([]byte, 36)
	header := new(bytes.Buffer)
	binary.Write(header, binary.LittleEndian, uint16(rec.channum))
	binary.Write(header, binary.LittleEndian, headerVersion)
	binary.Write(header, binary.LittleEndian, dataType)
	binary.Write(header, binary.LittleEndian, uint32(rec.presamples))
	binary.Write(header, binary.LittleEndian, uint32(len(rec.data)))
	binary.Write(header, binary.LittleEndian, rec.sampPeriod)
	binary.Write(header, binary.LittleEndian, float32(rec.peakValue)) // TODO: change to volts/arb
	nano := rec.trigTime.UnixNano()
	binary.Write(header, binary.LittleEndian, uint64(nano))
	binary.Write(header, binary.LittleEndian, uint64(rec.trigFrame))

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, rec.data)
	return header.Bytes(), buf.Bytes()
}

// this is run on package initialization
func init() {
	err := configurePubSocket()
	if err != nil {
		panic(fmt.Sprint("configurePubSocket error:", err))
	}
}
