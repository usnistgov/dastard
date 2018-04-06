package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
)

// SourceControl is the sub-server that handles configuration and operation of
// the Dastard data sources.
type SourceControl struct {
	simPulses    SimPulseSource
	activeSource DataSource
}

// FactorArgs holds the arguments to a Multiply operation
type FactorArgs struct {
	A, B int
}

// Multiply is a silly RPC service that multiplies its two arguments.
func (s *SourceControl) Multiply(args *FactorArgs, reply *int) error {
	*reply = args.A * args.B
	return nil
}

// ConfigureSimPulseSource configures the source of simulated pulses.
func (s *SourceControl) ConfigureSimPulseSource(args *SimPulseSourceConfig, reply *error) error {
	s.simPulses.Configure(args.nchan, args.rate, args.pedestal, args.amplitude, args.nsamp)
	*reply = nil
	return nil
}

// Start will identify the source given by sourceName and Sample then Start it.
func (s *SourceControl) Start(sourceName *string, reply *error) error {
	fmt.Println("Starting data source named ", sourceName)

	// Should select the activeSource using sourceName and error out if no match.
	s.activeSource = DataSource(&s.simPulses)
	s.activeSource.Start()

	*reply = nil
	return nil
}

// Stop stops the running data source, if any
func (s *SourceControl) Stop(dummy *string, reply *error) error {
	s.activeSource.Stop()
	*reply = nil
	return nil
}

// Set up and run a permanent JSON-RPC server.
func runRPCServer(portrpc int) {
	server := rpc.NewServer()

	// Set up objects to handle remote calls
	sourcecontrol := new(SourceControl)
	server.RegisterName("Arith", sourcecontrol)

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
	runRPCServer(PortRPC)

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
