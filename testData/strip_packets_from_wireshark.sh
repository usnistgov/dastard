#!/usr/bin/bash

# Take a wireshark packet sniffer data file ($INF) and generate an output
# file $OUTF containing only the raw UDP packet contents for the first 4 packets.
# I happen to know that the first 3 packets are external triggers, packets of length
# 0x260 with some wireshark junk of length 0x4c between them, and the first starting at
# 0x14a. Then the 4th packet is a longer packet (length 0x1060) of µMUX data.
# Use this knowledge to surgically remove these 4 packets and store in $OUTF.

# This script probably isn't good for re-use, but it can document how the test
# data file timer_packets.bin was generated.

INF=~/Downloads/timestamp_enabled_with_data.pcapng
OUTF=timer_packets.bin
rm -f $OUTF
dd bs=1 skip=0x14a count=0x260 if=$INF of=$OUTF
dd bs=1 skip=0x3f6 count=0x260 seek=0x260 if=$INF of=$OUTF
dd bs=1 skip=0x6a2 count=0x260 seek=0x4c0 if=$INF of=$OUTF
dd bs=1 skip=0x94e count=0x1060 seek=0x720 if=$INF of=$OUTF
