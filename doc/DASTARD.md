# Design of the DASTARD Data Acquisition Program

Memo author: Joe Fowler, NIST Boulder Labs

Date: March 2022

## Abstract

> The Data Acquisition System for Triggering And Recording Data (DASTARD) is a program written in Go to
> support the use of microcalorimeter spectrometers. It handles data from a variety of hardware sources,
> imposes trigger conditions, and distributes triggered records to data files (with a few choices of data
> format) and to a TCP port for plotting by an external program, Microscope. DASTARD is a complex, highly
> parallel program. This document is meant to summarize how it operates, with the goal of making future design
> changes easier to make.

## Responsibilities of DASTARD

DASTARD handles many responsibilities in the acquisition and analysis of microcalorimeter timestream data. 1.
DASTARD reads data from a data source such as an Abaco card for μMUX systems (see other examples in [Data
sources](#data-sources-for-dastard)). In some advanced cases that we haven’t really tested, it might even read
more than one source of the same type, such as multiple Abaco cards. 2. DASTARD appropriately handles unwanted
gaps in the data. 3. DASTARD de-multiplexes that raw data source into a set of data streams, one stream per
detector. 4. DASTARD applies triggering conditions to each data stream to determine when photon pulses appear
in the stream. When a pulse is found, it creates a triggered record of prescribed length (before and after the
trigger sample) from a continuous segment of the stream. 5. DASTARD records triggered records to files, in the
LJH or OFF formats. The LJH format records complete raw pulse records. The OFF format stores only linear
summaries of each record, including an optimally filtered pulse amplitude. 6. DASTARD also puts copies of each
triggered record onto a TCP port using the ZMQ Publisher paradigm, so that real-time plotting/analysis
programs such as microscope can use all triggered records, or those from a selection of channel numbers.

This memo is meant to summarize some internal implementation details of how DASTARD operates. It is not meant
to be a user’s guide, but an insider’s view. The goal is that Joe, Galen, or future DASTARD programmers have a
reference describing how. How the program is structured, how it performs certain key tasks. It is likely to
fall behind and omit some of the very latest details of DASTARD. I will hope that the program structure is
basically complete (as of early 2022) and changes are infrequent enough that we can still benefit even from a
document that’s slightly out of date.

## DASTARD as an RPC server

DASTARD replaces an earlier pair of programs
([`ndfb_server`](https://bitbucket.org/nist_microcal/nasa_daq/src/master/) and
[`matter`](https://bitbucket.org/nist_microcal/nasa_daq/src/master/)) that were written in C++. Each contains
both the code to perform any underlying DAQ activities and a GUI to permit users to control the programs. One
key design decision for DASTARD was to split the DAQ work and the GUI front-end into two separate programs.1
By splitting the DAQ work from the control GUI, we were able to write each in a language best suited to the
job. GUIs are really complicated, and writing a Qt5 GUI in Python is considerably easier than writing it in
C++ (though it is still far from easy).

The way the GUI(s) communicate with DASTARD is by having DASTARD operate a server for Remote Procedure Calls
(RPCs). The specific protocol used is JSON-RPC, in which the function calls, argument lists, and results are
represented as JSON objects. We are using [JSON-RPC version 1.0](https://www.jsonrpc.org/specification_v1). It
is implemented by a Go package, [net/rpc/jsonrpc](https://pkg.go.dev/net/rpc/jsonrpc).

The main DASTARD program (`dastard/dastard.go`) is simple. It performs one startup task then launches three
parallel tasks. The startup task uses the [viper configuration manager](http://github.com/spf13/viper) to read
a configuration file `$HOME/.dastard/config.yaml` that restores much of the configuration from the last run of
DASTARD. If a global configuration` /etc/dastard/config.yaml/` exists, it will be read before the user’s main
configuration. The parallel tasks are:

1. Start and keep open a [`log.Logger`](https://pkg.go.dev/log#Logger) in `dastard.problemLogger` that writes
to a rotating set of files in `$HOME/.dastard/logs/` 2. Launch `RunClientUpdater()` in a goroutine. It is used
to publish status updates on a ZMQ Publisher socket. The GUI client Dastard-commander and any other
control/monitoring clients learn the state of DASTARD by ZMQ-subscribing to this socket. 3. Call
`RunRPCServer()`. When it returns notify the `RunClientUpdater` goroutine to terminate, then end the program.

### The JSON-RPC server

Within `RunRPCServer` a new `SourceControl` object is created. It's in charge of configuring and running the
various supported data sources (e.g., an Abaco). It contains one each of a `LanceroSource`, `AbacoSource`,
`RoachSource`, `SimPulseSource`, and `TriangleSource`. These 5 specific types match the `DataSource` interface
and therefore offer a ton of identical methods like `Sample()`, `PrepareRun()`, `StartRun()`, and `Stop()`.
Each of these 5 sources also has configuration/control options specific to that type of data source. Each has
a stored state that is read (by viper) initially. A "new Dastard is running" message is sent to all active
clients (by pushing the message to `sourceControl.clientUpdates` channel, which the `RunClientUpdater`
goroutine is receiving from). The previous info about the data-writing state and the TES map (location) file
are loaded by viper. Also a new `MapServer` object is created to load and send TES position data.

After these setup tasks are completed, 2 goroutines are launched: 1. A heartbeat service. Using a
read-channel, this accumulates info about the time active and the number of bytes sent and the number read
from hardware (the latter might be less, in the case of dropped data being replaced by dummy filler data).
Using a `time.Tick` timer, the service wakes up every 2 seconds and sends the accumulated information as part
of an "ALIVE" message to any active clients. 2. An RPC connection handler. Creates a listener on tcp port 5500
(see `global_config.go` for where this is set) with `net.Listen("tcp", 5500)` and then waits for connections
with `listener.Accept()`. When received, a new goroutine is launched where codec is created by
`jsonrpc.NewServerCodec(...)` and we attempt to handle all RPC requests by calling
`server.ServeRequest(codec)` repeatedly until it errors. These requests are configured (before the listen
step) to be forwarded to the `sourceControl` or `mapServer` objects, as appropriate.

The RPC server supports the notion of an _active source_. At any one time, one or none of these sources may be
active. Some RPC requests are handled immediately. Others are queued up to be acted upon at the appropriate
phase in the data handling cycle, to prevent race conditions. The following can be handled immediately: -
`Start()`: argument says which of the 5 source types to activate. (Fails if any source is active.) - `Stop()`:
signals the active source to stop. (Fails if none is active.) - `ConfigureMixFraction()`: configure the TDM
mix if the Lancero source is active (errors otherwise). - `ReadComment()`: reads the `comment.txt` file.
Errors if no source is active, writing is not on, or the comment file cannot be read. - `SendAllStatus()`:
broadcast the maximal Dastard state information to any active clients (such as Dastard-commander).

The following can be handled immediately, but only if the corresponding source is not running: -
`ConfigureLanceroSource()` - `ConfigureAbacoSource()` - `ConfigureRoachSource()` - `ConfigureTriangleSource()`
- `ConfigureSimPulseSource()`

Most RPC commands only make sense when a source is active (often because they require the exact number of
channels to be known). An active source is delicate, so requests to change its state have to be saved for just
the right moment if we are to avert any race conditions. Requests are queued by calling `runLaterIfActive(f)`
where the argument `f` is a zero-argument, void-returning closure. The run-later function returns an error if
no source is running. If some source _is_ running, the closure goes into channel
`SourceControl.queuedRequests`. When a reply comes back on channel `SourceControl.queuedResults`, the
run-later function returns that reply. Requests that run delayed include: - `ConfigureTriggers` -
`ConfigureProjectorsBasis` - `ConfigurePulseLengths` - `WriteControl()`: starts/stops/pauses/unpauses data
writing. - `SetExperimentStateLabel()` - `WriteComment()` - `CoupleErrToFB()` - `CoupleFBToErr()`

## Data sources for DASTARD

All concrete data sources in DASTARD (such as `AbacoSource`) implement the `DataSource` interface, partly by
composing their source-specific data and methods with an `AnySource` object. That object implements the parts
of the interface that are not source-specific. The life-cycle of a data acquisition period looks like this.

When the RPC server gets a START request, it calls `Start(ds,...)` where the first argument is the specific
source of interest, for which all critical configuration has already been set. The function is found in
`data_source.go`. It calls, in order: 1. `ds.SetStateStarting()`: sets `AnySource.sourceState=Starting` but
with a Mutex to serialize access to the state (also `GetState()` and `SetStateInactive()` use the same Mutex).
1. `ds.Sample()`: runs a source-specific step that reads a certain amount of data from the source to determine
key facts such as the number of channels available and the data sampling rate. 1. `ds.PrepareChannels()`: now
that the number of channels and channel names and numbers are known/knowable, store the info in the
`AnySource`. There is an `AnySource.PrepareChannels`, but it's overridden by source-specific
`AbacoSource.PrepareChannels` or `LanceroSource.PrepareChannels`, because these two sources have more
complicated channel numbering possibilities. 1. `ds.PrepareRun()`: initialize run-related features, including
channel `ds.abortSelf` for knowing when to stop; channel `ds.nextBlock` for passing data chunks from the
source to the downstream processors; a `TriggerBroker`; tickers to regularly track the amount of data written,
write out external triggers, and monitor data drops (1, 1, and 10 seconds period). Also start one
`DataStreamProcessor` per channel (`process_data.go`), assigning the viper-stored trigger state as a starting
value. 1. `ds.RunDoneActivate()`: sets `AnySource.sourceState=Active` and increments a WaitGroup, again using
the Mutex to serialize access. 1. `ds.StartRun()`: calls a source-specific `StartRun()` method. It will
initiate data acquisition (some sources require a kind of "now go" command, which should be issued here). 1.
`go coreLoop(ds,...)`

The `coreLoop` (in `data_source.go`) starts with a `defer ds.RunDoneDeactivate` which will set
`AnySource.sourceState=Inactive` and decrements the WaitGroup, again using the Mutex to serialize access. It
then calls `ds.getNextBlock()` find the channel on which it needs to wait for new data blocks to appear. Then
it launches an infinite loop with select to wait on two distinct channels. One channel is the queued RPC
requests (the closure that need to wait for an appropriate place in the data handling); if one exists, we call
the closure. The other channel we wait on is the one returned by `getNextBlock`. If that channel was closed
_or_ includes a non-nil `block.err`, then source has to be stopped. But if a data block arrives with a nil
`err` component (the normal result), then we call `ds.ProcessSegments(block)`. More info at [Data flow in
DASTARD](#data-flow-in-dastard)) When that's done, we call `ds.getNextBlock()` again and repeat the loop at
the select statement. (In some sources this last call is extraneous, but for Lancero sources it is necessary
to prime the source again for reading again.)

When the RPC server gets a STOP request, it calls `AnySource.Stop()`. With proper Mutex locking, it checks for
status `Active`. (Status `Inactive` and `Stopping` are ignored and STOP has no effect. Status `Starting`
causes a panic.) If `Active`, set to `Stopping`. Close the `ds.abortSelf` channel to signal the `coreLoop`
that it's time to stop then wait on the WaitGroup to tell us that the data acquisition has actually stopped.
The closed abortSelf channel should be handled by the source and eventually result in a `ds.getNextBlock()`
returning a closed channel.

Once the stopping is complete, then the `SourceControl` object is back in the inactive state and ready to be
started again.

The source-specific `getNextBlock()` method is the one responsible for getting data. It starts any necessary
DAQ steps on the hardware and returns a channel where the data will be sent when ready. One key job is to
convert data from a source-specific to a source-independent form. The normal data flow is that the returned
channel is one that carries objects of the type `dataBlock`, a type that should make sense for data from any
possible source.

### TDM/Lancero data source

A `LanceroSource` reads out one or more `LanceroDevice` objects. Each `LanceroDevice` corresponds to a set of
device-special files like `/dev/lancero_user0`. (The use of 2+ devices has not yet been tested, and it in fact
causes DASTARD to panic in the main data loop. It's at least intended to be possible, however.)

The `Sample()` method checks each device. It grabs some data and finds the frame bits in the unaligned stream.
By analyzing these from the start of at least 2 successive frames, it figures out the number of columns and
channels in that device's TDM data block (which had previously been an undifferentiated data stream).

The `StartRun()` method has each Lancero device switch into data streaming mode (the Sample step switch
streaming on but then back off). It again looks for frame bits: this time not to count columns andchannels,
but just to discard bytes that are in the middle of a frame and to put the read pointer at the first byte of a
frame. Then `launchLanceroReader()` happens. That function launches a goroutine that starts a 50 ms ticker.
Then it loops forever, selecting on the `abortSelf` channel (which closes to indicate time to stop) and on the
ticker.

When the 50 ms ticker fires, we ask for an available buffer from the underlying card. If less than 3 frames
are available, they are left in the card's buffer and saved for the next tick. We test the data to verify that
the former numbers of channels and columns still hold (if not, releasing the buffer and apparently hoping for
better results next time; it seems like this ought to cause the data source to stop itself?). The data are
checked for drops (recognized by the buffer not being aligned with a frame, which admittedly will miss cases
where the dropped length is a multiple of the frame length). If any are dropped, we log that, and skip ahead
to the next full frame after the drop. Figure out how many frames are available, and demultiplex them into
`datacopies`, of type `[][]RawType` (1st index is channel, in lancero order; 2nd index is sample number).
Finally, we tell the driver to release the number of bytes we have demuxed and we put `datacopies` into the
`LanceroSource.buffersChan` (along with info about the time elapsed, time of last valid read from the device,
and whether a drop was detected). This appears on the channel returned by `getNextBlock()`.

`LanceroSource.getNextBlock` is a bit confusing. It launches a goroutine with an apparently infinite loop. But
in fact, the loop returns after the first message arrives on the `buffersChan` channel. Until then, it loops
to handle any requests to change the error-feedback mix, and to panic if 10 seconds elapse without the
expected message. When the expected message arrives, it's checked for being empty or for the message actually
being a closed channel (in those cases, it stops the Lancero device and reports out an error). If the message
has nonzero valid data, it's converted to a `dataBlock` by `LanceroSource.distributeData` and put onto the
channel `LanceroSource.nextBlock`. That channel has already been returned by the `getNextBlock` method.

The `LanceroSource.distributeData(msg)` function packages each deMUXed data slice into a `DataSegment` object,
which also adds the frame period, the first sample's frame serial number and sample time and a few other
housekeeping items. The collection of all such segments from all channels is called a `dataBlock`. The block
includes external trigger information, which is extracted here.

### Abaco data source

An `AbacoSource` (`abaco.go`) reads out one or more Abaco devices. The devices can be programmed to put data
into a large user-space ring buffer or to produce UDP packets for DASTARD to read. The ring buffer type is
older, and the UDP type is the newer version intended for long-term use. Either device produces data in a
packet format (defined and processed in `packets/packets.go`).

The `AbacoSource` owns slices of both `AbacoRing` and `AbacoUDPReceiver`. Both types implement the
`PacketProducer` interface. The slice of rings is filled at creation time in `NewAbacoSource()` by checking
for the existence of shared memory regions named `xdma*_c2h_0_description`. The UDP receivers, by contrast,
are not created until the source is configured in `AbacoSource.Configure()`, because the user has to choose
them from among the near-infinite space of host:port choices. After the configure call, the source maintains a
slice `producers` that can potentially have one or more sources of one or both types.

The `Sample()` method loops through each producer and launches `PacketProducer.samplePackets()` in a goroutine
on each, returning the results together on a single channel. For each packet received, the channels it
contains is checked. For each unique range of channels found in the sample stage, an `AbacoGroup` is created.
This is an object representing a consecutively numbered channels--packets always cover a single range of
consecutive channel numbers. Groups can be sorted according to the first number in each group (we'll verify
soon that no channel number appears in more than one group). In `NewAbacoGroup`, DASTARD creates a queue (a
slice to hold unprocessed packets) and a slice of `PhaseUnwrapper` objects (one per channel in the group).
After one result (one slice of packets) is received from each producer, the `as.groups` values are sorted and
checked to make sure no channel number is in multiple groups (returning an error from `Sample` if there are
overlaps). Finally, each group's data are checked in `AbacoGroup.samplePackets()` and the sample rate
computed. There we compare the rates to ensure all sources agree (returning error if not). The data queues are
emptied.

The `PrepareChannels` method computes channel names from the numbers in the one or more groups and makes a
slice of all numbers.

The `StartRun()` method tells each producer to dump its existing stored data and launches
`AbacoSource.readerMainLoop` in a goroutine. This, in turn, starts a 50 ms ticker and a 5 s timer (so we can
recognize packet timeouts). In an infinite loop it selects on the abort channel (exiting when that closes);
the timeout timer (also exiting if that fires); and the ticker. If the ticker ticks, all available packets are
read from all producers and handed to `AbacoSource.distributePackets()`. That simply checks each packet and
enqueues it on the appropriate group's data queue. Then each queue is checked to be sure there are no missing
packets (filling in null data if there are) and then that no queue's first packet has a global sequence number
earlier than any other (if it does, its "excess data" are dropped). If any group is found to have produced no
data here, DASTARD returns to the select to await more data. We find the number of data samples available on
the shortest group's queue and deMUX that data for each group with `AbacoGroup.demuxData()` into a new slice
of data slices, `datacopies`. Finally, an `AbacoBuffersType` is made containing all the deMUXed data and
housekeeping info about the timestamp and amount of data. That buffer is sent on the `as.buffersChan` channel,
the 5 second timeout is reset, and loop ends (returning to the select).

The `getNextBlock()` is similar to that of Lancero sources, but simpler because there are (as yet) no
configurations that can change while the Abaco source is running (akin to the mix settings of Lancero). The
method simply launches a goroutine that selects between a timeout that panics (after 100 of the 50 ms
data-read ticks have happened) and the expected arrival of a message on the `as.buffersChan` channel. That
method checks for the channel being closed, errors, or empty data payloads and in any of these cases closes
the `as.NextBlock` channel and returns. Otherwise, it creates a data block from the message in
`as.distributeData()` and puts that block on `as.NextBlock` (again closing `as.NextBlock` on error). That
channel is, of course, what `getNextBlock()` returns after launching the goroutine.

### ROACH2 data source

A `RoachSource` (`roach.go`) reads out one or more ROACH-2 software-defined radio systems. Each device is
monitored by a `RoachDevice` and sends its data as UDP packets. The packets are in a completely different form
from the Abaco packets, defined here in `type packetHeader` and parsed by `parsePacket(packet []byte)`. The
source has its devices bundle 100 ms of data and processes that amount in a bunch.

The `Sample()` method creates a device for each host:port pair that the user specifies. Unlike Abaco data, the
ROACH data does not include the information needed to determine the data sample rate, so the user also has to
specify that. Only the number of channels is learned by sampling the data packets. A slice of `PhaseUnwrapper`
objects is created (one per channel in the device).

The `StartRun()` method launches a long-lived goroutine which in turn launches one `RoachDevice.readPackets`
for each active device. Then it loops forever with a select that watches for the `abortSelf` channel to close
(closing the `rs.nextBlock` channel then ending the goroutine). Otherwise, data are received from a local
`nextBlock` channel from one of the active devices, whereupon the total bytes handled are updated and sent as
an occasional heartbeat and then the block is forwarded to `rs.nextBlock` for handling just as in any other
source.

We won't describe the processing in any more detail because it's rather similar in concept to the
`AbacoSource`, and because the Lancero and Abaco sources are much more widely used in current (2022)
environment.

### Test data sources

TODO: Explain the `TriangleSource` and the `SimPulseSource`. They both allow DASTARD to generate fake data for
testing and exploration purposes, even when no Lancero, Abaco, or ROACH2 hardware is available.

## Data flow in DASTARD

The previous section discusses how any specific data source's data is checked for sanity, demultiplexed, time
tagged, used to update housekeeping information, and packaged into a series of `dataBlock` objects that flow
in time-order through the `AnySource.nextBlock` channel. These blocks are meant to contain some ~50 ms of data
at a time (at least on average) for all channels currently running. As described far above, the next step (a
source-independent step) is to call `AnySource.ProcessSegments()` on the data block. First, the relevant data
types:

**Key data types** - The `dataBlock` contains only a slice of `DataSegment` objects (one per data stream) that
*are of equal length and completely synchronized, plus a slice saying when external triggers were found, an
*error container, and the length of the segments. - The `DataSegment` is a continuous, single-channel raw data
*buffer containing a limited piece of the data stream, plus info about raw-physical unit relationships, the
*first sample’s frame number and sample time and period. - The `DataStream` (`data_source.go`) models a
*continuous stream of data. It's basically identical to `DataSegment` in implementation, but the concept is
*different: the segment is a finite piece, while the stream is conceptually infinite (though only a limited
*portion is available at any one time). The object is a composition of a `DataSegment` plus the number of
*samples ever fed into it. - The `DataStreamProcessor` (`process_data.go`) contains all the state needed to
*decimate, trigger, write, and publish data from a single channel based on its `DataStream`.

When a data block is handed to `ProcessSegments(block)` (`data_source.go`), the following steps happen: 1.
Each of the source's `ds.processors` objects is given the appropriate data segment from the block, and
`DataStreamProcessor.processSegment` (details below) is called on the segment in a goroutine. A
`WaitGroup.Add()` is called before each, with a deferred `WaitGroup.Done()` 1. All processors as waited on
before proceeding (using `WaitGroup.Wait()`). Thus, the processing is done in parallel but all completed
before the next step. 1. A map is made containing the `triggerList` of new primary triggers for each
processor. The `ds.broker.Distribute()` is called with this map as an argument. It returns a similar map,
containing a slice of new secondary trigger points for each processor to handle. 1. Each processor that has
any secondary triggers, if any, will have `dsp.processSecondaries()` on the secondary trigger points. Again,
this is done in one goroutine per processor, with a `WaitGroup` used to synchronize before moving to the next
step. 1. A loop over processors is made. It cleans up by marking each segment as processed; trimming each
processor's data stream (saving only 1 record worth of old data) with `dsp.TrimStream()`. Also, every 20th
time through, it calls `dsp.Flush()` to flush any open output files (but this is done out of phase, so only
~1/20th of all processors are flushed in each iteration). 1. We `HandleExternalTriggers` and `HandleDataDrop`.
If writing is active (not paused), we send a NUMBERWRITTEN message to all listening clients.

Each `DataStreamProcessor` contains tons of information: a channel's name and number; the number of samples
and pre-trigger samples to be used in making triggered records; a `DataStream`;  a basis and noise-optimal
projectors for making linear approximations to a record (for OFF files); info about the last stream index
where triggers were made; a `DecimateState`; a `TriggerState`; and a `DataPublisher`.

In `DataStreamProcessor.processSegment(seg)`, the steps are: 1. `dsp.DecimateData(seg)` to downsample the
segment by an integer factor (either by averaging or dropping samples). 1. `dsp.stream.AppendSegment(seg)` to
append the new data to the processor's stream. 1. `dsp.TriggerData()` to compute _primary_ triggers only. 1.
`dsp.AnalyzeData()` to analyze any primary triggered records. Computes pretrigger mean and pretrigger delta
(?), pulse average and pulse RMS for each record, and (if projectors exist) the record's model coefficients
(projections) and residual RMS. Store these results in the `DataRecord` object (`data_source.go`). 1.
`dsp.DataPublisher.PublishData(rec)` for the primary triggers.

In `DataStreamProcessor.processSecondaries(secondaryTrigList)`, the same last two steps are performed but this
time on secondary triggers. We define _Primary triggers_ = due to the data in this stream. _Secondary_ = due
to trigger coupling of some kind and a primary trigger happening in this processor's "source" channel.)

### Triggering in DASTARD

The above functions are all in `process_data.go` except for the trigger step. Triggering is so complicated, it
lives separately in `triggering.go` with the complex `TriggerState` object and some relevant methods. The
`dsp.TriggerData()` step does the following in sequence: 1. If `dsp.EdgeMulti`, then any other triggers will
be ignored (they don't work well together). The rest of this list will be skipped. 1. Compute edge triggers
and append them to the (empty) list of triggered records. 1. Compute level triggers and append them to the
list of triggered records. 1. Compute auto triggers and append them to the list of triggered records. 1.
(Noise triggers would go here, but they are not currently implemented.) 1. Create a `TriggerList` object
containing all information about the full collection of triggered records. Store as `dsp.lastTrigList`. 1.
Return a slice of `*DataRecord` objects.  Each `DataRecord` corresponds to one primary trigger and contains a
slice with the raw data, trigger time and frame index info, and placeholders for the analyzed quantities.

For edge-multi-triggers, `dsp.edgeMultiTriggerComputeAppend(records)` is called and the rest of the list is
skipped. The records are used to make a `TriggerList`, store it, and return a `[]*DataRecord` (same as last
two steps in the above list).

TODO: Fuller description of EMTs. Hold off writing for now, because this will change soon.

### Writing files in DASTARD

Each `DataStreamProcessor` contains (is composed with) a `DataPublisher` (`publish_data.go`). Its job is to
publish data records on certain ZMQ PUBlisher channels and to write them to disk. When `PublishData(rec
*[]DataRecord)` is called: 1. The records are sent on the full record channel `PubRecordsChan` (if it's
non-nil). 1. The records are sent on the record summary channel `PubSummariesChan` (if it's non-nil). 1. If
writing is paused, or if no output writing types are active, return. 1. If LJH 2.2 writing is active, then
`LJH22.WriteRecord(rec)` is called on each (first writing the header if this is the first record written to
that output file). 1. If LJH 3 writing is active, then `LJH3.WriteRecord(rec)` is called on each (first
writing the header if this is the first record written to that output file). 1. If OFF writing is active, then
`OFF.WriteRecord(rec)` is called on each (first writing the header if this is the first record written to that
output file). 1. Update the count of records written.

A single full record channel `PubRecordsChan` is shared by all processors, so records from all data streams go
on this same channel. It is set up for use by `startSocket()` (`publish_data.go`), which opens a ZMQ PUB
socket on port `Ports.Trigs` (set to be the base port+2=5502 in `global_config.go`), sets the send high-water
mark to 10 and launches a goroutine. That routine loops until `PubRecordsChan` is closed (at which point it
closes the ZMQ socket and returns). If data records arrive, it converts each to a message with the
`messageRecords` function (returns `[][]byte`, a 2-slice with the message header and the raw pulse record as
the two elements). This two-part ZMQ message is then sent over the ZMQ socket, repeating until all records
found on the channel have been processed and sent.

The `PubSummariesChan` is similarly unique and shared by all processors. It is set up and operates just like
the records channel except that: - It sends out data on a different ZMQ PUB socket on port `Ports.Summaries`
(base+3=5503). - It converts a record to a message with the `messageSummaries` function, which returns a
2-part message with a different header, and with the second part being the linear projection coefficients (the
values that would be stored in OFF files).

The published data messages are described in detail in `BINARY_FORMATS.md` in this directory.

The actual writing to file(s) is straightforward and serialized, so we won't describe it further. Code appears
in `ljh/ljh.go` (LJH 2.2 and 3.0 files) and `off/off.go` (OFF files).
