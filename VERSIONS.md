## DASTARD Versions
**0.2.1** October 2018 (in progress)
* Make mix command accept an array of new mix values and report all back to clients.
* Make SimPulse configuration accept an array of amplitudes to generate multiple pulse sizes.

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
