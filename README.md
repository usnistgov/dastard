# Dastard
[![Build Status](https://travis-ci.org/usnistgov/dastard.svg?branch=master)](https://travis-ci.org/usnistgov/dastard)

A data acquisition framework for NIST transition-edge sensor (TES) microcalorimeters. Designed to replace the earlier programs `ndfb_server` and `matter` (see their [bitbucket repository](https://bitbucket.org/nist_microcal/nasa_daq)).

## Installation
Requires Go version 1.16 or higher because [gonum](http://gonum.org/v1/gonum/mat) requires it. Dastard is tested automatically on versions 1.16 and LATEST (as of May 2022, Go version 1.18.2 is the most recent).

### Ubuntu 20, 18.04 and 16.04
One successful installation of the dependencies looked like this. Before pasting the following, be sure to run some
simple command as sudo first; otherwise, the password entering step will screw up your multi-line paste.
```
# Dependencies (can skip if git is already installed)
sudo apt-get -y update
sudo apt-get install -y libsodium-dev libczmq-dev git gcc pkg-config
# install go
sudo add-apt-repository -y ppa:longsleep/golang-backports
sudo apt-get -y update
sudo apt-get -y install golang-go

# Install Dastard as a development copy
DASTARD_DEV_PATH=$(go env GOPATH)/src/github.com/usnistgov
mkdir -p $DASTARD_DEV_PATH
cd $DASTARD_DEV_PATH
git clone https://github.com/usnistgov/dastard
cd dastard

# Build, then try to run the local copy
make all    # implies "make test build install"
./dastard --version

# Check whether the GOPATH is in your bash path. If not, update ~/.bashrc to make it so.
# This will fix the current terminal AND all that run ~/.bashrc in the future, but not
# any other existing terminals (until you do "source ~/.bashrc" in them).
source update-path.sh

# Now that PATH includes go's bin, try to run the installed copy
dastard --version
```

The following seem to be out-of-date and apply only to Ubuntu 16.04 LTS, but just in case they
are useful to you, here are the old instructions.
 ```
sudo add-apt-repository -y 'deb http://download.opensuse.org/repositories/network:/messaging:/zeromq:/git-stable/xUbuntu_16.04/ ./'
cd ~/Downloads
wget http://download.opensuse.org/repositories/network:/messaging:/zeromq:/git-stable/xUbuntu_16.04/Release.key
sudo apt-key add - < Release.key
sudo apt-get -y update
sudo apt-get install -y libsodium-dev libczmq-dev git
```

Get go version 1.13 or higher, with MacPorts or homebrew, or by direct download. For Ports, assuming
that MacPorts is already installed, it's simple:

### MacOS dependencies

#### Homebrew
`brew install go pkg-config czmq libsodium`

#### Macports
`port install go czmq libsodium-dev pkg-config`



### Also install these

* Install microscope https://github.com/usnistgov/microscope
* Install dastard_commander https://github.com/usnistgov/dastardcommander


### And boost your UDP receive buffer

This only applies to users who will need µMUX, or any data source that eventually sends UDP packets to Dastard. The size of the standard Linux receive buffer for network sockets is only 208 kB, which is not nearly sufficient and leads to dropped packets. We recommend increasing the buffer size to 64 MB. In Linux, that buffer size parameter is called `net.core.rmem_max`. In Mac OS X it seems to be called `net.inet.udp.recvspace`. For one-time testing, this means you increase the buffer with:

```bash
sudo sysctl -w net.core.rmem_max=67108864
```

If that works as intended, you can make the configuration permanent (persist after rebooting) by adding the following line to `/etc/sysctl.conf`

`net.core.rmem_max=67108864`

That's only for Linux machines. OS X doesn't seem to have such a file, but might instead require a startup script, possibly in `/Library/StartupItems/`.

## Purpose

DASTARD is the Data Acquisition System for Triggering, Analyzing, and Recording Data. It is to be used with TES microcalorimeter arrays and the NIST Time-Division Multiplexed (TDM) readout or--in the future--the Microwave Multiplexed readout.

Assuming that the TDM readout system is properly configured first (beyond the scope of this project), one can use Dastard to:
1. read the high-rate data stream;
1. search it for x-ray pulse triggers;
1. generate "triggered records" of a fixed length;
1. store these records to disk in the LJH data format;
1. "publish" the full records to a socket for other programs to plot or use as they see fit; and
1. compute important derived quantities from these records.

### Goals, or, why did we need Dastard?

Some reasons to replace the earlier data acquisition programs with Dastard in Go:

1. To permit simple operation of more than one data source of the same type under one program (e.g., multiple uMUX boxes or multiple PCI-express interfaces to the TDM crate). With previous DAQ programs, multiple PCI-e cards required multiple invocations of the data server, which were difficult to coordinate.
1. To employ multi-threading to parallelize over multiple cores, and do so in a language with natural concurrency constructs. (This was attempted in Matter, but it never worked because of the algorithm used there to generate group triggering.)
1. To separate the GUI from the "back-end" code. This keeps the core functions from having to share time with the GUI event loop, and it allows for more natural use of "external commanding", such as we require at synchrotron beamlines.
1. To break the rigid assumption that channels fall into error/feedback pairs. For microwave MUX systems, this will be quite useful.
1. To use code testing from the outset and ensure code correctness.
1. To start with a clean slate, so that we can more easily add new features, such as
   * Generalize the group trigger (each channel can receive triggers from its own list of sources). This will help with cross-talk studies and with the Kaon project HEATES.
   * Allow writing of group-triggered ("secondary") pulses to different files.
   * Generate "variable-length records" for certain high-rate analyses. (This is still hypothetical.)

Some goals for DASTARD, in addition to these reasons:

* Scale up to operate even with future, larger arrays.
* No longer require C++ expertise to work on DAQ software, and to use only Python for the GUI portions.

## Project structure

DASTARD is a back-end program written in Go. It handles the "server" aspects of `ndfb_server`, as well as the triggering and data-recording duties of `matter`. It is complemented by two GUI-based projects: a control GUI and a data plotter.

* [dastardcommander](https://github.com/usnistgov/dastardcommander) is a Python-Qt5 GUI to control Dastard.
* [microscope](https://github.com/usnistgov/microscope) is a Qt-based plotting GUI to plot microcalorimeter pulse records, discrete Fourier transforms, and related data. It is written in C++ and contains code from [matter](https://bitbucket.org/nist_microcal/nasa_daq/)

We envision future control clients other than dastard-commander. They should enable commanding from, for example, the beamline control system of a synchrotron.


### Contributors and acknowledgments

* Joe Fowler (@joefowler or joe.fowler@nist). Sept 2017-present
* Galen O'Neil (@ggggggggg or oneilg@nist). May 2018-present

For all email address, replace "@nist" with "@nist.gov".

Many key concepts in Dastard were adapted from the programs `ndfb_server`, `xcaldaq_client`, and `matter`, written by Dastard's authors and by contributors from both NIST Boulder Laboratories and NASA Goddard Space Flight Center.
