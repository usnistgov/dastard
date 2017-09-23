#!/usr/bin/env python
import requests
import json

# From https://haisum.github.io/2015/10/13/rpc-jsonrpc-gorilla-example-in-golang/
# This isn't working completely.
# Tried with module requests from http://docs.python-requests.org/en/latest/
# and with urllib2, and both produced connection aborted: BadStatusLine errors.

def rpc_call(url, method, args):
    print("url <%s>"%url)
    data = json.dumps({
        'id': 0,
        'method': method,
        'params': [args]
    }).encode()
    print "Data <%s>"%data

    req = requests.get(url, data, headers={'Content-Type': 'application/json'})
    return req.json()

url = 'http://localhost:4234/rpc/'
args = {'A': 1, 'B': 2}
print rpc_call(url, "Arith.Multiply", args)
