package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
)

// See https://parthdesai.me/articles/2016/05/20/go-rpc-server/ blog for
// more helpful discussion.
// For the jsonrpc see https://gist.github.com/nicerobot/8954764

// Sniff packets thus:
// sudo tcpdump -nnvXSs 0 -i lo0 port 4234

func handler(rw http.ResponseWriter, req *http.Request) {
	fmt.Println("Handler h was called with request: ")
	fmt.Println(req.URL)
	response := fmt.Sprintf("1")
	fmt.Println("Response: ", response)
	fmt.Fprintf(rw, response)
}

func main() {

	useRPC := true
	if useRPC {
		arith := new(Arith)

		server := rpc.NewServer()

		// The following lines are silly--they simply answer the question of
		// how the server handles having TWO registered objects of the same type.
		// The answer appears to be that the second one registered is ignored.
		// The net/rpc docs say that it's actually an error to register 2 of a type.
		// arith2 := new(Arith)
		// arith2.ncalls = 100
		// server.Register(arith2)

		server.Register(arith)
		server.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
		listener, e := net.Listen("tcp", ":4234")
		if e != nil {
			log.Fatal("listen error:", e)
		}
		for {
			if conn, err := listener.Accept(); err != nil {
				log.Fatal("accept error: " + err.Error())
			} else {
				log.Printf("new connection established\n")
				go server.ServeCodec(jsonrpc.NewServerCodec(conn))
			}
		}
	} else {
		http.HandleFunc("/", handler)
		log.Fatal(http.ListenAndServe(":4234", nil))

	}
}
