## DASTARD Versions

**0.2.12** March 2022
* Add `doc/` directory with `DASTARD.md` to explain the internal design of the code (issue 261).
* Make Lancero driver's ring buffer be 256 MB instead of 32 MB (issue 258).
* Redesign triggering so all-channel sync point is explicit (not hidden in `TriggerBroker`) (issues 235, 255).
* Redesign edge-multi-triggering for clarity and to fix problems (issues 256, 259, 260).

**0.2.11** October 12, 2021
* Fix test for exact Float64 equality on data rate from different chan groups (issue 248).
* Work on biased phase unwrapping (issue 250).

**0.2.10** May 27, 2021
* Packet data can appear as UDP instead of in shared memory buffer.
* Fix bug in closing the file that logs data drops (issue 239).
* Improve logging of info when packets are dropped (issue 241).
* Improve speed limit on UDP data packet handline (issue 242).
* Fix ROACH2 source with latest Dastard design (issue 246).

**0.2.9** March 11, 2021
* Fix Lancero source: fill in channel groups as 1 group per column.
* Use new `goczmq` API for setting socket options (issue 230).
* Keep separate counts of the data rate _from_ the hardware, and data _processed_ (after dropped packets are
  replaced by artificial data, the latter might be larger). Send both in heartbeat ("ALIVE") message (issue 229).
* New agreement that Abaco will supply exactly [0, 2π) in the full range of whatever sized int data
  it generates. So if int32, use the highest 16 bits (issue 227).
* Allow Abaco sources to change the time after which the phase unwrapping resets (issue 228).
* Fix heartbeat ("ALIVE") message to have `Running=false` after sources stop (issue 232).
* Have Travis Continuous Integration system deploy a compiled binary to github (issue 236).

**0.2.8** October 23, 2020
* Remove idea of rows/columns from Dastard (used only in `LanceroSource`). Use chan groups otherwise (issue 214).
* Lancero source can have more flexible channel numbers, such as 12043 for card 1, column 2, row 43 (issue 212).
* Testing of the flexible channel numbering.
* Add support for TAG TLVs in Abaco packet data (issue 216).
* Ignore unknown TLV types in Abaco packet data (issue 217).
* Fix bug that prevented stopping and restarting Abaco (issue 218).
* Much improved speed of processing Abaco packets (issue 220).

**0.2.7** September 10, 2020
* Use package `zmq4`, a pure Go implementation of ZMQ.
* Simplified testing on Travis because of that.
* Then revert to `gozmq` because `zmq4` was dropping messages (issue 192).
* Read Abaco packets correctly when fractional packets are in ring buffer (issue 188).
* Fix CurrentTime client message to be valid JSON (issue 194).
* Added program Bahama to generate fake Abaco packet-style data (issues 196, 200).
* Fix problem where ZMQ 2-part messages were sent w/o checking for errors (issue 198).
* Handle Abaco packets from separate channel groups as independent (issue 190).
* Fill in fake data when any Abaco packets are missing (issue 202).
* Fix MaxSamples value in OFF file headers (issue 197).
* Log when Abaco packets are missing (issue 204).
* Channel numbering is no longer consecutive (issue 191).

**0.2.6** April 2, 2020
* Change handling of data drops.
* Generate OFF files v0.3.0, with pretrigger Deltas.
* Usability features improved.
* Abaco source with packets.

**0.2.5** September 5, 2019
* Abaco data source works for streaming data (not packets).
* Reset triggering memory on changing trigger settings.
* Fix `goczmq` API for socket options.
* Fix handling of, and absence of, map files.

**0.2.4** June 17, 2019
* Read Cringe global variable data file.
* Lancero LSYNC stored by Cringe; read by Dastard.
* Refactored WriteControl object.
* Use pointers to `mat.Dense` as recommended by its author.
* Write detector positions in OFF file headers; new OFF version.

**0.2.3** April 19, 2019
* ROACH source works.

**0.2.2** December 19, 2018
* External triggers

**0.2.1** December 7, 2018
* Make mix command accept an array of new mix values and report all back to clients.
* Make SimPulse configuration accept an array of amplitudes to generate multiple pulse sizes.
* Timestamp RPC commands SetExperimentStateLabel when received, not when processed.
* Fix problems with simulated pulse data when read in MASS.
* Get record lengths correct when starting a source.

**0.2.0** October 5, 2018
* Redesign the internals of Dastard to eliminate data races between RPC services
  that try to change data processing and the actual processing. Use more channels.

**0.1.2** September 28, 2018
* Uses an internal buffered channel to queue Lancero data for processing so that
  we can survive 1-second-plus delays in processing (writing seems to be the
  particular problem).

**0.1.1** August 13, 2018
* Demonstrated OFF file writing with triggering.

**0.1.0** July 13, 2018
* Ready for acquiring LJH (and maybe OFF) files with triggering.

**0.0.2** July 2018
* Still improving to reach useful configuration

**0.0.1** July 2018
* Fix irregular data rates for simulated sources due to latencies.

**0.0.0** Development through July 2018.  
* Trying to build a working system.

Version number is set in source file `global_config.go`.
