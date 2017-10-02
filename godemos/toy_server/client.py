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
        response = self._codec.loads(response.decode())

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
        self.spinBoxes = [self.spinBox, self.spinBox_2, self.spinBox_3]

        self.client = JSONClient(("localhost", 4444))
        # params = {"A":13,"B":9}
        # for a in range(25):
        #     params["A"] = a
        #     print client.call("Arith.Multiply", params)

    def reject(self):
        print("Rejected")
        self.close()

    def start(self):
        print("Start")
        print("Spin boxes: %s"%([b.value() for b in self.spinBoxes]))
        for i,b in enumerate(self.spinBoxes):
            params = {"channum":i, "fact": b.value()}
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


if __name__ == "__main__":
    app = QtGui.QApplication(sys.argv)
    window = MyApp()
    window.show()
    sys.exit(app.exec_())
