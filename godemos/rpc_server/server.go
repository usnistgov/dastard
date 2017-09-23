package main

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
)

// See https://parthdesai.me/articles/2016/05/20/go-rpc-server/ blog for
// more helpful discussion.

// Sniff packets thus:
// sudo tcpdump -nnvXSs 0 -i lo0 port 4234

func main() {
	arith := new(Arith)
	// rpc.Register(arith)
	// rpc.HandleHTTP()
	server := rpc.NewServer()
	server.Register(arith)                   // register by natural name Arith
	server.RegisterName("Arithmetic", arith) // register by alternate name, too.
	server.HandleHTTP("/gorpc", "/debug")
	l, e := net.Listen("tcp", ":4234")
	if e != nil {
		log.Fatal("listen error:", e)
	}
	http.Serve(l, nil)
}
