package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
)

// Set up and run a permanent JSON-RPC server.
func rpcServer(portrpc int) {
	server := rpc.NewServer()

	// Set up objects to handle remote calls
	simPulses := new(SimPulseSource)
	server.RegisterName("Arith", simPulses)

	// Now launch the connection handler and accept connections.
	server.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
	port := fmt.Sprintf(":%d", portrpc)
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	for {
		if conn, err := listener.Accept(); err != nil {
			log.Fatal("accept error: " + err.Error())
		} else {
			log.Printf("new connection established\n")
			go server.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}
}

func main() {
	fmt.Printf("Port %d\n", PortRPC)
	fmt.Printf("Port %d\n", PortStatus)
	rpcServer(PortRPC)

	// NChan := 4
	// source := new(SimPulseSource)
	// source.Configure(NChan, 200000.0, 5000.0, 10000.0, 65536)
	//
	// ts := DataSource(source)
	// ts.Sample()
	// ts.Start()
	// allOutputs := ts.Outputs()
	// abort := make(chan struct{})
	//
	// dataToPub := make(chan []*DataRecord, 500)
	// go PublishRecords(dataToPub, abort, pubRecPort)
	//
	// broker := NewTriggerBroker(NChan)
	// go broker.Run(abort)
	//
	// // Launch goroutines to drain the data produced by the DataSource.
	// for chnum, ch := range allOutputs {
	// 	dsp := NewDataStreamProcessor(chnum, abort, dataToPub, broker)
	// 	dsp.Decimate = false
	// 	dsp.DecimateLevel = 3
	// 	dsp.DecimateAvgMode = true
	// 	dsp.LevelTrigger = true
	// 	dsp.LevelLevel = 5500
	// 	dsp.NPresamples = 200
	// 	dsp.NSamples = 1000
	// 	go dsp.ProcessData(ch)
	// }
	//
	// // Have the DataSource produce data until graceful stop.
	// go func() {
	// 	for {
	// 		if err := ts.BlockingRead(abort); err == io.EOF {
	// 			fmt.Printf("BlockingRead returns EOF\n")
	// 			return
	// 		} else if err != nil {
	// 			fmt.Printf("BlockingRead returns Error\n")
	// 			return
	// 		}
	// 	}
	// }()
	//
	// // Take data for 4 seconds, stop, and wait 2 additional seconds.
	// time.Sleep(time.Second * 44)
	// fmt.Println("Stopping data acquisition. Will quit in 2 seconds")
	// close(abort)
	// time.Sleep(time.Second * 2)
}
