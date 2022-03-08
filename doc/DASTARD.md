# Design of the DASTARD Data Acquisition Program

Memo author: Joe Fowler, NIST Boulder Labs

Date: March 2022

## Abstract

> The Data Acquisition System for Triggering And Recording Data (DASTARD) is a program written in Go to support the use of microcalorimeter spectrometers. It handles data from a variety of hardware sources, imposes trigger conditions, and distributes triggered records to data files (with a few choices of data format) and to a TCP port for plotting by an external program, Microscope. DASTARD is a complex, highly parallel program. This document is meant to summarize how it operates, with the goal of making future design changes easier to make.

## Responsibilities of DASTARD

DASTARD handles many responsibilities in the acquisition and analysis of microcalorimeter timestream data.
1. DASTARD reads data from a data source such as an Abaco card for μMUX systems (see other examples in [Data sources](#Datasources)). In some advanced cases that we haven’t really tested, it might even read more than one source of the same type, such as multiple Abaco cards.
2. DASTARD appropriately handles unwanted gaps in the data.
3. DASTARD de-multiplexes that raw data source into a set of data streams, one stream per detector.
4. DASTARD applies triggering conditions to each data stream to determine when photon pulses appear in the stream. When a pulse is found, it creates a triggered record of prescribed length (before and after the trigger sample) from a continuous segment of the stream.
5. DASTARD records triggered records to files, in the LJH or OFF formats. The LJH format records complete raw pulse records. The OFF format stores only linear summaries of each record, including an optimally filtered pulse amplitude.
6. DASTARD also puts copies of each triggered record onto a TCP port using the ZMQ Publisher paradigm, so that real-time plotting/analysis programs such as microscope can use all triggered records, or those from a selection of channel numbers.

This memo is meant to summarize some internal implementation details of how DASTARD operates. It is not meant to be a user’s guide, but an insider’s view. The goal is that Joe, Galen, or future DASTARD programmers have a reference describing how. How the program is structured, how it performs certain key tasks. It is likely to fall behind and omit some of the very latest details of DASTARD. I will hope that the program structure is basically complete (as of early 2022) and changes are infrequent enough that we can still benefit even from a document that’s slightly out of date.

## DASTARD as an RPC server

DASTARD replaces an earlier pair of programs (`ndfb_server` and `matter`) that were written in C++. Each contains both the code to perform any underlying DAQ activities and a GUI to permit users to control the programs. One key design decision for DASTARD was to split the DAQ work and the GUI front-end into two separate programs.1 By splitting the DAQ work from the control GUI, we were able to write each in a language best suited to the job. GUIs are really complicated, and writing a Qt5 GUI in Python is considerably easier than writing it in C++ (though it is still far from easy).

The way the GUI(s) communicate with DASTARD is by having DASTARD operate a server for Remote Procedure Calls (RPCs). The specific protocol used is JSON-RPC, in which the function calls, argument lists, and results are represented as JSON objects.

The main DASTARD program (dastard/dastard.go) is simple. It performs one startup task then launches three parallel tasks. The startup task uses the viper configuration manager (http://github.com/spf13/viper) to read a configuration file `$HOME/.dastard/config.yaml` that restores much of the configuration from the last run of DASTARD. If a global configuration` /etc/dastard/config.yaml/` exists, that will be read before the user’s main configuration. The parallel tasks are:

1. Start and keep open a log.Logger in dastard.problemLogger that writes to a rotating set of files in $HOME/.dastard/logs/
2. Launch `RunClientUpdater()` in a goroutine. It is used to publish status updates on a ZMQ Publisher socket. The GUI client Dastard-commander and any other control/monitoring clients learn the state of DASTARD by ZMQ-subscribing to this socket.
3. Call `RunRPCServer()`. When it returns notify the `RunClientUpdater` goroutine to terminate,
then end the program.

## Data sources for DASTARD
### TDM/Lancero data source
### Abaco data source
### ROACH2 data source
### Test data sources
## Data flow in DASTARD
### Triggering in DASTARD
### Writing files in DASTARD
