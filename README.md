# Dastard
[![Test+Build at GitHub Actions](https://github.com/usnistgov/dastard/actions/workflows/go.yml/badge.svg?branch=master)](https://github.com/usnistgov/dastard/actions)

A data acquisition program for NIST transition-edge sensor (TES) microcalorimeters. Designed to replace the earlier programs `ndfb_server` and `matter` (see their [bitbucket repository](https://bitbucket.org/nist_microcal/nasa_daq)).

## Installation
**Requires Go version 1.17 or higher** (released August 2021) because [gonum](http://gonum.org/v1/gonum/mat) requires it. Dastard is tested automatically on versions 1.17 and _stable_ (as of May 2024, Go version 1.22.3 is the most recent stable version).

**We recommend always using the `Makefile` to build Dastard.** That's not a typical go usage, but we have a simple trick built into the `Makefile` that allows it to execute `go build` with linker arguments to set values of two global variables. By this step, we are able to get the git hash and the build date of the current version to be known inside Dastard. Hooray! The lesson is always use one of the following:
```bash
# To build locally and run that copy
make build && ./dastard
```
or
```bash
# To install as $(go env GOPATH)/bin/dastard, which generally means $HOME/go/bin/dastard
make install   # also implies make build
dastard
```

**There was temporarily a problem with Go 1.20 and TDM systems** If you are using TDM systems with the Lancero device, beware that we had trouble making the device read correctly with Go 1.20 (as of April 20, 2023). This turns out to be the result of a bug in Go 1.20, surprisingly. We believe this problem was fixed in May 2023 (Dastard v0.2.19) with a workaround, but we leave this note here just in case you notice problems we didn't. Also, it should be fixed in Go 1.21. Certainly, for data sources other than Lancero, we are unaware of _any_ Go versions that cause problems.


### Ubuntu 22.04, 20.04, 18.04, and (maybe?) 16.04

One successful installation of the dependencies looked like this. Before pasting the following, be sure to run some
simple command as sudo first; otherwise, the password entering step will screw up your multi-line paste.
```bash
# Dastard dependencies
sudo apt-get -y update
sudo apt-get install -y libsodium-dev libzmq3-dev git gcc pkg-config

# Install go
sudo add-apt-repository -y ppa:longsleep/golang-backports
sudo apt-get -y update
sudo apt-get -y install golang-go
go version

# If you need lancero-TDM, install go 1.19,
# then ensure that version go-1.19 is the preferred version in /usr/bin
# If the above reports version 1.19, or if you don't need lancero-TDM, then skip what follows
# If the above reports version 1.18 or lower, obviously change the instances of "1.19" in what follows.
sudo apt-get -y install golang-1.19-go
pushd /usr/bin
sudo ln -sf ../lib/go-1.19/bin/go go
sudo ln -sf ../lib/go-1.19/bin/gofmt gofmt
popd


# Install Dastard as a development copy
# (These lines should have an equivalent "go install" like maybe "go install gitub.com/usnistgov/dastard@latest"??)
DASTARD_DEV_PATH=$(go env GOPATH)/src/github.com/usnistgov
mkdir -p $DASTARD_DEV_PATH
cd $DASTARD_DEV_PATH
git clone https://github.com/usnistgov/dastard
cd dastard

# Build, then try to run the local copy
# Using the Makefile is preferred, because it's the only way we're aware of to get the git hash and build date
# embedded in the built binary file. Run the new binary and check its version output.
make build && ./dastard --version

# Check whether the GOPATH is in your bash path. If not, update ~/.bashrc to make it so.
# This will fix the current terminal AND all that run ~/.bashrc in the future, but not
# any other existing terminals (until you do "source ~/.bashrc" in them).
source update-path.sh

# Now that PATH includes go's bin, try to install and run the installed copy
make install
dastard --version
```

Be sure to add `~/go/bin` to your `PATH` environment variable, or replace `~/go` with the results of your specific result when you run `go env GOPATH`.

The following seem to be out-of-date and apply only to Ubuntu 16.04 LTS, but just in case they are useful to you, here are the old instructions.
 ```bash
sudo add-apt-repository -y 'deb http://download.opensuse.org/repositories/network:/messaging:/zeromq:/git-stable/xUbuntu_16.04/ ./'
cd ~/Downloads
wget http://download.opensuse.org/repositories/network:/messaging:/zeromq:/git-stable/xUbuntu_16.04/Release.key
sudo apt-key add - < Release.key
sudo apt-get -y update
sudo apt-get install -y libsodium-dev libczmq-dev git
```

### Gentoo Linux

The following seemed to work on a NASA balloon flight computer in March 2023:
```bash
USE="static-libs -systemd" emerge --ask net-libs/zeromq dev-libs/libsodium
emerge --ask dev-vcs/git dev-lang/go
mkdir -p $(go env GOPATH)/bin/
```

After this, follow the Ubuntu instructions starting with "# Install Dastard as a development copy". Make sure that the `update-path.sh` script worked and properly updated your `PATH` variable. That script hasn't been tested in Gentoo, as far as I know.


### MacOS dependencies (Homebrew or MacPorts)

As appropriate, use one of

```bash
brew install go zmq libsodium pkg-config
```

or

```bash
sudo port install go zmq libsodium-dev pkg-config
```



### Also install these companion programs

* Install microscope https://github.com/usnistgov/microscope
* Install dastard_commander https://github.com/usnistgov/dastardcommander


### And boost your UDP receive buffer

This only applies to users who will need µMUX, or any data source that eventually sends UDP packets to Dastard. The size of the standard Linux receive buffer for network sockets is only 208 kB, which is not nearly sufficient and leads to dropped packets. We recommend increasing the buffer size to 64 MB. In Linux, that buffer size parameter is called `net.core.rmem_max`. In Mac OS X it seems to be called `net.inet.udp.recvspace`. For one-time testing, this means you increase the buffer with:

```bash
sudo sysctl -w net.core.rmem_max=67108864
```

If that works as intended, you can make the configuration permanent (i.e., it will persist after rebooting) if
you add the following line to `/etc/sysctl.conf`

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

We envision future control clients other than Dastard-commander. They should enable commanding from, for example, the beamline control system of a synchrotron.

Futher reading:
* [DASTARD.md](doc/DASTARD.md): info about Dastard internals (very technical!)
* [BINARY_FORMATS.md](doc/BINARY_FORMATS.md): info about the binary data packets sent by Dastard (very technical!)

### Configuration
DASTARD caches its configuration via the [Viper](https://github.com/spf13/viper) "complete configuration solution for Go applications". You can find your user file at `$HOME/.dastard/config.yaml`. It's human-readable, though you should normally never need to read or edit it. If you have configuration problems and cannot find another way out, it is okay to delete this file (or hide it temporarily in, for example, `/tmp/`). If you want some kind of global defaults that you want to persist even if that file is deleted, it is possible to create a global `/etc/dastard/config.yaml` file with a subset of the values you want to set by default.

But again, this is expert usage! You should not normally need to touch or look at the configuration file. It's there for internal use to allow settings to persist from one run of Dastard to another.

### Contributors and acknowledgments

* Joe Fowler (@joefowler or joe.fowler@nist). Sept 2017-present
* Galen O'Neil (@ggggggggg or oneilg@nist). May 2018-present

For all email address, replace "@nist" with "@nist.gov".

Many key concepts in Dastard were adapted from the programs `ndfb_server`, `xcaldaq_client`, and `matter`, written by Dastard's authors and by contributors from both NIST Boulder Laboratories and NASA Goddard Space Flight Center.
