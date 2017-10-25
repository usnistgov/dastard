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
	// Structure of the message header is
	// 16 bits: channel number
	//  8 bits: header version number
	//  8 bits: code for data type (0-1 = 8 bits; 2-3 = 16; 4-5 = 32; 6-7 = 64; odd=uint; even=int)
	// 32 bits: record length (# of samples)
	// 64 bits: trigger time
	// 64 bits: trigger frame #

	const headerVersion = uint8(0)
	const dataType = uint8(3) // will want to assert about RawType somehow?

	header := make([]byte, 24)
	binary.LittleEndian.PutUint16(header[0:], uint16(rec.channum))
	header[2] = headerVersion
	header[3] = dataType
	binary.LittleEndian.PutUint32(header[4:], uint32(len(rec.data)))
	nano := rec.trigTime.UnixNano()
	binary.LittleEndian.PutUint64(header[8:], uint64(nano))
	binary.LittleEndian.PutUint64(header[16:], uint64(rec.trigFrame))
	fmt.Printf("% x\n", header)

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, rec.data)
	return header, buf.Bytes()
}
