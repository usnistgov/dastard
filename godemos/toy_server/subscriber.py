#!/usr/bin/env python

import zmq

# Socket to talk to server
context = zmq.Context()
socket = context.socket(zmq.SUB)

port = 4445
socket.connect ("tcp://localhost:%s" % port)
topicfilter = "data chan 0"
socket.setsockopt(zmq.SUBSCRIBE, topicfilter)
while True:
    message = socket.recv()
    print message
