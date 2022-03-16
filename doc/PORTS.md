# DASTARD TCP PORTS

(Last revision: 17 March 2021.)

DASTARD uses a series of TCP ports for communications with its GUI control
clients (such as [DASTARD-Commander](https://github.com/usnistgov/dastard-commander)) and to publish triggered pulse data for plotting
([Microscope](https://github.com/usnistgov/microscope)). See also the [UDP ports](#DASTARD-UDP-PORTS)

These ports are numbered sequentially from a *base port* number. By default,
this number is **BASE=5500**. We might allow this number to be set at the
DASTARD command-line, but for now it's a constant.  The TCP ports are:

* **5500** (base+0): **Control**. JSON-RPC port for controlling DASTARD.  (JSON-RPC = "Remote Procedure Calls" specificed by JSON data format). Message format is defined by [json-rpc version 1.0](http://www.jsonrpc.org/specification_v1).
* **5501** (base+1): **Status**. ZMQ PUB port where DASTARD reports its status to all control GUIs.
* **5502** (base+2): **Pulses**. ZMQ PUB port where DASTARD puts all pulse records. Subscribe by 4-byte channel number. These are for Microscope to use, so it can plot data.
* **5503** (base+3): **Secondary records**. ZMQ PUB port, same as BASE+2, except that here we put only the secondary triggered records (i.e from a group trigger).
* **5504** (base+4): **Pulse summaries**. ZMQ PUB port. Just has summary info and model fit coefficients.

### JSON-RPC commands (BASE+0)

Hmm. Should document these.

### Status messages (BASE+1)
Format is a text message-key (as a ZMQ frame) then a status block in JSON format. The messages are meant to be adequate to inform all Dastard control clients (the `dastard-commander` GUI, or others) everything they need to know about the Dastard internal state. Message keys include:

* **ALIVE**: a "heartbeat" message, whether data source is active, time and MB of data since previous ALIVE message. Expect <5 seconds apart.
* **STATUS**: what data source is active; idling or running;  What # of rows, columns, channels, samples, and pre-trigger samples.
* **TRIGGER**: contains the complete trigger configuration (publish only when it changes). An efficiency: send only 1 copy of each unique state, along with a list of the channel numbers that are in that specific state.
* **TRIGCOUPLING**: whether FB->Error or Error->FB trigger coupling is active, or neither.
* **STATELABEL**: the current "experiment state".
* **SIMPULSE**: contains the configuration of the Simulated Pulse data source.
* **TRIANGLE**: contains the configuration of the Triangle Wave data source.
* **LANCERO**: contains the configuration of the Lancero data source (e.g., which cards to use, fiber mask, etc.).
* **ABACO**: contains the configuration of the Abaco data source (e.g., which ring buffers to use).
* **TRIGGERRATE**: how many triggers have been counted (array-wide) over some duration, plus the clock time of last checked sample.
* **NUMBERWRITTEN**: counts how many records have been written to file.
* **DATADROP**: counts data frames dropped from an active data source (since previous message).
* **EXTERNALTRIGGER**: counts how many external triggers have been seen (since previous message).
* **TESMAP**: characterizes the entire TES array geometry.
* **TESMAPFILE**: names the TES array map file being used.
* **MIX**: TDM mixing state. Like TRIGGER, publish all values that match as a block of identically mixed channels.
* **WRITING**: contains output file information (type, filename, writing status stop/go/pause) (publish on change).
* **CHANNELNAMES**: a list of the unique channel names.

### Primary and secondary pulse records (BASE+2 and BASE+3)

Each message on these ports consists of a single pulse record. The first 2 bytes are an int16 channel number, so that programs can subscribe to specific channels. The message format is found in file BINARY_FORMATS.md

### Pulse summaries (BASE+4)

Each message on these ports contains _summaries_ of a single pulse record. The first 2 bytes are an int16 channel number, so that programs can subscribe to specific channels. The message format is found in file BINARY_FORMATS.md

# DASTARD UDP PORTS

DASTARD can receive data packets placed onto UDP. Currently, it makes assumptions about the source of the data based on the port to which the datagrams are sent:

* UDP ports 4000-4299 for µMUX systems
* UDP ports 4400-4699 for TDM systems (still under development)
