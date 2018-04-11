package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"
)

// SourceControl is the sub-server that handles configuration and operation of
// the Dastard data sources.
type SourceControl struct {
	simPulses SimPulseSource
	triangle  TriangleSource
	// TODO: Add sources for Lancero, ROACH, Abaco
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
func (s *SourceControl) ConfigureSimPulseSource(args *SimPulseSourceConfig, reply *bool) error {
	fmt.Printf("ConfigureSimPulseSource: %d chan, rate=%.3f\n", args.Nchan, args.SampleRate)
	if args.Nchan < 1 {
		return fmt.Errorf("ConfigureSimPulseSource requests %d channels, needs at least 1", args.Nchan)
	}
	err := s.simPulses.Configure(args)
	*reply = (err == nil)
	fmt.Printf("Result is %t and state: %d chan, rate=%.3f\n", *reply, s.simPulses.nchan, s.simPulses.sampleRate)
	return nil
}

// Start will identify the source given by sourceName and Sample then Start it.
func (s *SourceControl) Start(sourceName *string, reply *bool) error {
	name := strings.ToUpper(*sourceName)
	switch name {
	case "SIMPULSESOURCE":
		s.activeSource = DataSource(&s.simPulses)
	// case "TRIANGLESOURCE":
	// 	s.activeSource = DataSource(&s.triangle)
	// TODO: Add cases here for LANCERO, ROACH, ABACO, etc.
	default:
		return fmt.Errorf("Data Source \"%s\" is not recognized", *sourceName)
	}

	fmt.Printf("Starting data source named %s\n", *sourceName)
	go Start(s.activeSource)
	*reply = true
	return nil
}

// Stop stops the running data source, if any
func (s *SourceControl) Stop(dummy *string, reply *bool) error {
	fmt.Printf("Stopping data source\n")
	if s.activeSource != nil {
		s.activeSource.Stop()
		s.activeSource = nil
		*reply = true
	}
	return nil
}

// Set up and run a permanent JSON-RPC server.
func runRPCServer(portrpc int) {
	server := rpc.NewServer()

	// Set up objects to handle remote calls
	sourcecontrol := new(SourceControl)
	server.Register(sourcecontrol)

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
}
