## DASTARD Versions

**0.4.0** June 10, 2025-
* Build in (optional) use of a ClickHouse database to store run+channel info.

**0.3.7** August 5, 2025
* Reorganize the sub-modules to live in "internal" (so their API isn't exposed outside Dastard).
* Synchronize disk writes occasionally, and in parallel across multiple channels (issues 353, 354).
* Add a working `Dockerfile` for potential future use.
* More verbose info when called at command line with `-version`.

**0.3.6** May 12, 2025
* Make Abaco data be considered unsigned (issue 364).
* Panic at a point where it's easier to explain why, if 2 abaco groups find unequal sample rates (issue 359).
* Fix crashes when rate is high and a packet is dropped (issue 360).
* Add feature where EMT retrigger can create one record, not just 0 or 2 records (issue 366).

**0.3.5** April 22, 2025
* Fix bug that was writing zeros for the external trigger (issue 362).

**0.3.4** August 23, 2024
* Update all package dependencies (issue 347).
* Send which types of files are active in the report about writing data (issue 343).
* Rename bad Makefile variable `LDFLAGS` -> `GOLINKFLAGS` (issue 350).
* When no config file exists, set `Nsamples` to a nonzero value, preventing some crashes (issues 355, 356).

**0.3.3** May 28, 2024
* Make publisher more responsive: don't block data processors waiting on disk flush.
* Construct all file paths using `path/filepath` to get OS-specific details right (issue 342).

**0.3.2** February 9, 2024
* Make external trigger resolution 64x finer (issue 335).
* Add subframe info to LJH and OFF headers; use subframe language throughout code (issue 337).
* Save client msg to ~/.dastard/logs/updates.log (not terminal); cute ASCII bouncer at terminal (issue 338).

**0.3.1** February 3, 2024
* Add configuration to invert an arbitrary subset of Abaco channels (issue 330).
* Process UDP packets containing external trigger information (issue 331).
* Add configuration to allow biased unwrapping with Roach (issue 328).

**0.3.0** August 10, 2023
* Add RPC command to store raw, untriggered data to temporary npy/npz files (issue 320).

**0.2.19** May 26, 2023
* Fix unchecked error in starting Abaco sources (issue 321).
* Workaround for bug in Go library `os.OpenFile` (issue 324).
* Fix fact that the workaround was yielding zeros from lancero source (issue 326).

**0.2.18** April 24, 2023
* Panic if run on Go 1.20 or higher with the lancero-TDM readout (issue 315).
* Return Lancero driver's ring buffer to 32 MB (undo issue 258); check return values, panic if needed (issue 316).

**0.2.17** March 7, 2023
* Add Gentoo instructions; add static compilation target to Makefile (issue 302).
* Switch to GitHub Actions for testing and deployment (issue 305, 308).
* Require Go 1.17 or higher (PR 306).
* Remove source files that were used only for go 1.16 and earlier (issue 310).

**0.2.16** February 2, 2023
* Make ROACH2 firmware for HOLMES team work (issue 297).
* Add a veto possibility to the auto-trigger (issue 300).

**0.2.15** November 29, 2022
* Add a `udpdump` main program to read a few packets from an Abaco UDP source (issue 290).
* Add some profiling features to Dastard main program (as command-line options).
* Redesign "legacy triggers" so they don't unintentionally overlap (issue 293).

**0.2.14** September 26, 2022
* Add features to test use of multiple Abaco cards at once (like Tomcat-1k).
* Fix crashes found when data rates (in sim data) are very low and some chan don't trigger (issue 277).
* Make Edge Multi Triggers stop crashing Dastard every time they are switched on (issue 279).
* Using VScode's golang code checker, found and fixed deprecations and other bad practices (PR 282).
* Change ZMQ library from goczmq to pebbe/zmq4 (issues 253, 281).
* Clarify in the README what the config.yaml file is for and in the file itself (issue 285).
* Fix bug in the main reader loop of an `AbacoUDPReceiver`; caused crashes when loop tries to end (issue 288).

**0.2.13** June 1, 2022
* Fix crashing problem when you start with Edge Multi Triggers on (issue 271).
* Update all dependencies with latest upstream versions.
* Add way to turn on/off µMUX rescaling separate from phase unwrapping (for non-µMUX UDP sources).

**0.2.12** March 18, 2022
* Add `doc/` directory with `DASTARD.md` to explain the internal design of the code (issue 261).
* Make Lancero driver's ring buffer be 256 MB instead of 32 MB (issue 258).
* Redesign triggering so all-channel sync point is explicit (not hidden in `TriggerBroker`) (issues 235, 255).
* Redesign edge-multi-triggering for clarity and to fix problems (issues 256, 259, 260).
* Add general-purpose "group trigger" (issue 247).
* Use `unsafe.Slice` (new in Go 1.17) to convert between slice types (issue 267).

**0.2.11** October 12, 2021
* Fix test for exact Float64 equality on data rate from different chan groups (issue 248).
* Work on biased phase unwrapping (issue 250).

**0.2.10** May 27, 2021
* Packet data can appear as UDP instead of in shared memory buffer.
* Fix bug in closing the file that logs data drops (issue 239).
* Improve logging of info when packets are dropped (issue 241).
* Improve speed limit on UDP data packet handling (issue 242).
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
