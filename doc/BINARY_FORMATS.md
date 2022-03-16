# Binary Serialization Formats for DASTARD Data Structures

## Binary format for triggered data records

12/21/2017 (updated 9/2/2020)

Triggered records are published on a ZMQ PUB socket. Primary triggers appear on
port *BASE*+2, and secondary (cross-talk) triggers appear on port *BASE*+3.

### Message Version 0

Dated 4/19/2018. Triggered data records go into a 2-frame ZMQ message. The first frame contains
the header, which is 36 bytes long. The second frame is the raw record data, which
is of variable length and packed in little-endian byte order.
The header also contains little-endian values:

* Byte 0 (2 bytes): channel number
* Byte 2 (1 byte):  header version number (0 in this version)
* Byte 3 (1 byte):  data type code (see below)
* Byte 4 (4 bytes): samples before trigger
* Byte 8 (4 bytes): samples in record
* Byte 12 (4 bytes): sample period in seconds (float)
* Byte 16 (4 bytes): volts per arb (float)
* Byte 20 (8 bytes): trigger time (nanoseconds since 1 Jan 1970)
* Byte 28 (8 bytes): trigger frame index

Because the channel number makes up the first 2 bytes, ZMQ subscriber sockets can
subscribe selectively to only certain channels.

Data type code: so far, only uint16 and int16 are allowed.

* 0 = int8
* 1 = uint8
* 2 = int16
* 3 = uint16
* 4 = int32
* 5 = uint32
* 6 = int64
* 7 = uint64


## Binary format for triggered data summaries


Summaries of triggered records are published on a ZMQ PUB socket on port *BASE*+4. The summaries
assume that the data have been projected onto a low-dimensional linear subspace (a basis).

### Message Version 0

Dated 9/2/2020. Triggered data records go into a 2-frame ZMQ message. The first frame contains
the header, which is 36 bytes long. The second frame is the raw record data, which
is of variable length and packed in little-endian byte order.
The header also contains little-endian values:

* Byte 0 (2 bytes): channel number
* Byte 2 (2 byte):  header version number (0 in this version)
* Byte 4 (4 bytes): samples before trigger
* Byte 8 (4 bytes): samples in record
* Byte 12 (4 bytes): pretrigger mean value
* Byte 16 (4 bytes): peak value (pretrigger mean subtracted first)
* Byte 20 (4 bytes): pulse RMS (pretrigger mean subtracted first)
* Byte 24 (4 bytes): pulse average (pretrigger mean subtracted first)
* Byte 28 (4 bytes): residual standard deviation (basis vectors projected out first)
* Byte 32 (8 bytes): trigger time (nanoseconds since 1 Jan 1970)
* Byte 40 (8 bytes): trigger frame index

Because the channel number makes up the first 2 bytes, ZMQ subscriber sockets can
subscribe selectively to only certain channels.

The second frame consists of the projection coefficients, from the linear projection into the basis.
The coefficients are float64, and the size of the second frame should be 8 times the number of coefficients.
