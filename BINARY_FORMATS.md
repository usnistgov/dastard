# Binary Serialization Formats for DASTARD Data Structures

## Binary Format for Triggered Data records

10/25/2017

Triggered records are published on a ZMQ PUB socket. Primary triggers appear on
port *BASE*+2, and secondary (cross-talk) triggers appear on port *BASE*+3.

### Packet Version 0

Dated 10/25/2017. Packets consist of a 2-frame ZMQ message. The first frame contains
the header, which is 24 bytes long. The second frame is the raw record data, which
is of variable length and packed in little-endian byte order.
The header also contains little-endian values:

* 2 bytes: channel number
* 1 byte:  header version number (0 in this version)
* 1 byte:  data type code (see below)
* 4 bytes: record length (# of samples)
* 8 bytes: trigger time (nanoseconds since 1 Jan 1970)
* 8 bytes: trigger frame #

Because the channel number makes up the first 2 bytes, ZMQ subscriber sockets can
subscribe selectively to only certain channels.

Data type code:

* 0 = int8
* 1 = uint8
* 2 = int16
* 3 = uint16 (*so far, only this type is allowed*)
* 4 = int32
* 5 = uint32
* 6 = int64
* 7 = uint64
