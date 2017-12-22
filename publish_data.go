package dastard

import (
	"bytes"
	"encoding/binary"
	"fmt"

	czmq "github.com/zeromq/goczmq"
)

// PublishRecords publishes one data packet per DataRecord received on its input to
// a ZMQ PUB socket. It terminates when abort channel is closed.
func PublishRecords(dataToPub <-chan []*DataRecord, abort <-chan struct{}, portnum int) {
	hostname := fmt.Sprintf("tcp://*:%d", portnum)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return
	}
	defer pubSocket.Destroy()

	for {
		select {
		case <-abort:
			return
		case records := <-dataToPub:
			for _, rec := range records {
				header, signal := packet(rec)
				pubSocket.SendFrame(header, czmq.FlagMore)
				pubSocket.SendFrame(signal, czmq.FlagNone)
			}
		}
	}
}

func packet(rec *DataRecord) ([]byte, []byte) {
	// Structure of the message header is defined in BINARY_FORMATS.md
	// 16 bits: channel number
	//  8 bits: header version number
	//  8 bits: code for data type (0-1 = 8 bits; 2-3 = 16; 4-5 = 32; 6-7 = 64; odd=uint; even=int)
	// 32 bits: # of pre-trigger samples
	// 32 bits: record length (# of samples)
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
	binary.Write(header, binary.LittleEndian, uint32(rec.channum)) // TODO: change to pretrig length
	binary.Write(header, binary.LittleEndian, uint32(len(rec.data)))
	binary.Write(header, binary.LittleEndian, float32(rec.peakValue)) // TODO: change to sample period
	binary.Write(header, binary.LittleEndian, float32(rec.peakValue)) // TODO: change to volts/arb
	nano := rec.trigTime.UnixNano()
	binary.Write(header, binary.LittleEndian, uint64(nano))
	binary.Write(header, binary.LittleEndian, uint64(rec.trigFrame))

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, rec.data)
	return header.Bytes(), buf.Bytes()
}
