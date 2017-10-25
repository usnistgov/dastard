#!/usr/bin/env python

import zmq, struct
import numpy as np
import pylab as plt

# Socket to talk to server
context = zmq.Context()
socket = context.socket(zmq.SUB)

port = 32002
socket.connect ("tcp://localhost:%s" % port)
channum = 3
topicfilter = struct.pack("<H", channum)
socket.setsockopt(zmq.SUBSCRIBE, topicfilter)
i = 0
plt.ioff()
while True:
    header = socket.recv()
    message = socket.recv()
    print struct.unpack("<HbblQq", header)
    if i%12 == 0:
        plt.clf()
    i += 1

    record = np.fromstring(message, dtype=np.uint16)
    plt.plot(record, label="Message %d"%i)
    plt.draw()
    if i > 5:
        plt.legend()
        plt.show(block=False)
