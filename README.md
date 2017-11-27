# Dastard
[![Build Status](https://travis-ci.org/usnistgov/dastard.svg?branch=master)](https://travis-ci.org/usnistgov/dastard)

NIST transition-edge sensor (TES) data acquisition framework. Designed to replace `ndfb_server` and `matter` (see [bitbucket repository](https://bitbucket.org/nist_microcal/nasa_daq)).

## Goals

Some reasons to replace the earlier data acquisition programs with DASTARD in Go:

1. Allow simple operation of more than one data source of the same type (e.g., multiple uMUX boxes or multiple PCI-express interfaces to the TDM crate).
1. Use multi-threading to parallelize over multiple cores, and do so in a language with natural concurrency constructs. (This was attempted in Matter but never worked because of the algorithm used there to generate group triggering.)
1. Separate the GUI from the "back-end" code. This keeps the core functions from sharing time with the GUI event loop, and it allows for more natural use of "external commanding", such as we require at beamlines.
1. Break the required pairing of channels into error/feedback pairs.
1. Use code testing from the outset to ensure code correctness.
1. With a clean slate, we can more easily add new features, such as
   * Generalize the group trigger (each channel can receive triggers from its own list of sources). This will help with cross-talk studes.
   * Allow writing of group-triggered ("secondary") pulses to different files.
   * Generate "variable-length records" for certain high-rate analyses. (This is still hypothetical.)

Some goals for DASTARD, in addition to these reasons:

* Scale up to operate even with future, larger arrays.
* No longer require C++ expertise to work on DAQ software.

## Project structure 

DASTARD will be a no-GUI back-end program written in Go. It will handle the "server" aspects of `ndfb_server`, and the triggering and data-recording duties of `matter`. It will be complemented by two GUI-based projects: a control GUI and a data plotter. These are not yet underway--we'll add more about them as they progress.
