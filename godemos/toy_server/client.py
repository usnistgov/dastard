#!/usr/bin/env python
import requests
import json
import itertools, socket
import sys

# From https://haisum.github.io/2015/10/13/rpc-jsonrpc-gorilla-example-in-golang/
# This isn't working completely.
# Then tried https://groups.google.com/forum/#!topic/golang-nuts/6sCvbBaZvtI
# Tried with module requests from http://docs.python-requests.org/en/latest/
# and with urllib2, and both produced connection aborted: BadStatusLine errors.

from PyQt4 import QtCore, QtGui, uic

qtCreatorFile = "client.ui"

Ui_MainWindow, QtBaseClass = uic.loadUiType(qtCreatorFile)

class JSONClient(object):

    def __init__(self, addr, codec=json):
        self._socket = socket.create_connection(addr)
        self._id_iter = itertools.count()
        self._codec = codec

    def _message(self, name, params):
        return dict(id=next(self._id_iter),
                    params=[params],
                    method=name)

    def call(self, name, params):
        request = self._message(name, params)
        print "jwf sending request :<%s>"%request
        id = request.get('id')
        msg = self._codec.dumps(request)
        self._socket.sendall(msg.encode())

        # This will actually have to loop if resp is bigger
        response = self._socket.recv(4096)
        try:
            response = self._codec.loads(response.decode())
        except ValueError as e:
            print "The decoder raised error: ", e
            raise e

        if response.get('id') != id:
            raise Exception("expected id=%s, received id=%s: %s"
                            %(id, response.get('id'),
                              response.get('error')))

        if response.get('error') is not None:
            print "Yikes! Reponse is: ", response
            raise Exception(response.get('error'))

        return response.get('result')

    def close(self):
        self._socket.close()

class MyApp(QtGui.QMainWindow, Ui_MainWindow):
    def __init__(self):
        QtGui.QMainWindow.__init__(self)
        Ui_MainWindow.__init__(self)
        self.setupUi(self)
        self.buttonBox.accepted.connect(self.close)
        self.buttonBox.rejected.connect(self.reject)
        self.startButton.clicked.connect(self.start)
        self.stopButton.clicked.connect(self.stop)
        self.spinBoxes = [self.spinBox, self.spinBox_2, self.spinBox_3, self.spinBox_4]
        self.spinBox.valueChanged.connect(self.updatef1)
        self.spinBox_2.valueChanged.connect(self.updatef2)
        self.spinBox_3.valueChanged.connect(self.updatef3)
        self.spinBox_4.valueChanged.connect(self.updatef4)
        self.client = JSONClient(("localhost", 4444))

    def reject(self):
        print("Rejected")
        self.close()

    def start(self):
        print("Start")
        print("Spin boxes: %s"%([b.value() for b in self.spinBoxes]))
        for i,b in enumerate(self.spinBoxes):
            params = {"Channum":i, "Fact": b.value()}
            result = self.client.call("DataChannels.UpdateFact", params)
            print "Result: ", result

        params = {}
        result = self.client.call("DataChannels.Start", params)
        print "Result: ", result

    def stop(self):
        print("Stop")
        params = {}
        result = self.client.call("DataChannels.Stop", params)
        print "Result: ", result

    def update(self, chan, newval):
        print "Update: ", chan, newval
        params = {"Channum":chan, "Fact":newval}
        result = self.client.call("DataChannels.UpdateFact", params)
        print "Result: ", result

    def updatef1(self, n): self.update(0, n)
    def updatef2(self, n): self.update(1, n)
    def updatef3(self, n): self.update(2, n)
    def updatef4(self, n): self.update(3, n)


if __name__ == "__main__":
    app = QtGui.QApplication(sys.argv)
    window = MyApp()
    window.show()
    sys.exit(app.exec_())
