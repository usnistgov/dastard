package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"

	czmq "github.com/zeromq/goczmq"
)

// SourceControl is the sub-server that handles configuration and operation of
// the Dastard data sources.
type SourceControl struct {
	simPulses SimPulseSource
	triangle  TriangleSource
	// TODO: Add sources for Lancero, ROACH, Abaco
	activeSource DataSource

	status         SourceControlStatus
	statusMessages chan<- StatusMessage
}

type SourceControlStatus struct {
	Running    bool
	SourceName string
	Nchannels  int
	// TODO: maybe Ncol, Nrow, bytes/sec data rate...?
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

// ConfigureTriangleSource configures the source of simulated pulses.
func (s *SourceControl) ConfigureTriangleSource(args *TriangleSourceConfig, reply *bool) error {
	fmt.Printf("ConfigureTriangleSource: %d chan, rate=%.3f\n", args.Nchan, args.SampleRate)
	err := s.triangle.Configure(args)
	*reply = (err == nil)
	fmt.Printf("Result is okay=%t and state={%d chan, rate=%.3f}\n", *reply, s.triangle.nchan, s.triangle.sampleRate)
	return err
}

// ConfigureSimPulseSource configures the source of simulated pulses.
func (s *SourceControl) ConfigureSimPulseSource(args *SimPulseSourceConfig, reply *bool) error {
	fmt.Printf("ConfigureSimPulseSource: %d chan, rate=%.3f\n", args.Nchan, args.SampleRate)
	err := s.simPulses.Configure(args)
	*reply = (err == nil)
	fmt.Printf("Result is okay=%t and state={%d chan, rate=%.3f}\n", *reply, s.simPulses.nchan, s.simPulses.sampleRate)
	return err
}

// Start will identify the source given by sourceName and Sample then Start it.
func (s *SourceControl) Start(sourceName *string, reply *bool) error {
	name := strings.ToUpper(*sourceName)
	switch name {
	case "SIMPULSESOURCE":
		s.activeSource = DataSource(&s.simPulses)
		s.status.SourceName = "SimPulses"

	case "TRIANGLESOURCE":
		s.activeSource = DataSource(&s.triangle)
		s.status.SourceName = "Triangles"

	// TODO: Add cases here for LANCERO, ROACH, ABACO, etc.

	default:
		return fmt.Errorf("Data Source \"%s\" is not recognized", *sourceName)
	}

	fmt.Printf("Starting data source named %s\n", *sourceName)
	go Start(s.activeSource, s)
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

		s.status.Running = false
		s.status.SourceName = ""
		s.status.Nchannels = 0
	}
	s.publishStatus()
	return nil
}

func (s *SourceControl) publishStatus() {
	if payload, err := json.Marshal(s.status); err == nil {
		s.statusMessages <- StatusMessage{"STATUS", payload}
	}
}

// StatusMessage carries the messages to be published on the status port.
type StatusMessage struct {
	tag     string
	message []byte
}

func runStatusPublisher(messages <-chan StatusMessage, portstatus int) {
	hostname := fmt.Sprintf("tcp://*:%d", portstatus)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return
	}
	defer pubSocket.Destroy()

	for {
		select {
		case update := <-messages:
			pubSocket.SendFrame([]byte(update.tag), czmq.FlagMore)
			pubSocket.SendFrame(update.message, czmq.FlagNone)
		}
	}

}

// Set up and run a permanent JSON-RPC server.
func runRPCServer(messageChan chan<- StatusMessage, portrpc int) {
	server := rpc.NewServer()

	// Set up objects to handle remote calls
	sourcecontrol := new(SourceControl)
	sourcecontrol.statusMessages = messageChan
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
	messageChan := make(chan StatusMessage)
	go runStatusPublisher(messageChan, PortStatus)
	runRPCServer(messageChan, PortRPC)
}
