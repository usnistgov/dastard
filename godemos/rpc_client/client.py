#!/usr/bin/env python
import requests
import json

# From https://haisum.github.io/2015/10/13/rpc-jsonrpc-gorilla-example-in-golang/
# This isn't working completely.
# Then tried https://groups.google.com/forum/#!topic/golang-nuts/6sCvbBaZvtI
# Tried with module requests from http://docs.python-requests.org/en/latest/
# and with urllib2, and both produced connection aborted: BadStatusLine errors.

import itertools, socket

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

# def rpc_call(url, method, args):
#     print("url <%s>"%url)
#     data = json.dumps({
#         'id': 0,
#         'method': method,
#         'params': [args]
#     }, separators=(',',':')).encode()
#     data = "{'params':[{'A':9,'B':13}],'id':0,'method':'Arith.Multiply'}"
#     print "Data <%s>"%data
#
#     req = requests.get(url, data, headers={'Content-Type': 'application/json'})
#     return req.json()
#
# url = 'http://localhost:4234/rpc/'
# args = {'A': 1, 'B': 2}
# print rpc_call(url, "Arith.Multiply", args)

client = JSONClient(("localhost",4234))
params = {"A":13,"B":9}
for a in range(25):
    params["A"] = a
    print client.call("Arith.Multiply", params)
