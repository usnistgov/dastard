package dastard

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"
	"time"
)

// SourceControl is the sub-server that handles configuration and operation of
// the Dastard data sources.
type SourceControl struct {
	simPulses SimPulseSource
	triangle  TriangleSource
	// TODO: Add sources for Lancero, ROACH, Abaco
	activeSource DataSource

	status        SourceControlStatus
	clientUpdates chan<- ClientUpdate
}

// SourceControlStatus the status that SourceControl reports to clients.
type SourceControlStatus struct {
	Running    bool
	SourceName string
	Nchannels  int
	Nsamples   int
	Npresamp   int
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

// SizeObject is the RPC-usable structure for ConfigurePulseLengths to change pulse record sizes.
type SizeObject struct {
	Nsamp int
	Npre  int
}

// ConfigurePulseLengths is the RPC-callable service to change pulse record sizes.
func (s *SourceControl) ConfigurePulseLengths(sizes SizeObject, reply *bool) error {
	fmt.Printf("ConfigurePulseLengths: %d samples (%d pre)\n", sizes.Nsamp, sizes.Npre)
	if s.activeSource == nil {
		return fmt.Errorf("No source is active")
	}
	err := s.activeSource.ConfigurePulseLengths(sizes.Nsamp, sizes.Npre)
	*reply = (err == nil)
	s.status.Npresamp = sizes.Npre
	s.status.Nsamples = sizes.Nsamp
	fmt.Printf("Result is okay=%t\n", *reply)
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
	go func() {
		err := Start(s.activeSource)
		if err == nil {
			s.status.Running = true
			s.status.Nchannels = s.activeSource.Nchan()
			s.broadcastUpdate()
		}
	}()
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
	s.broadcastUpdate()
	return nil
}

func (s *SourceControl) broadcastUpdate() {
	if payload, err := json.Marshal(s.status); err == nil {
		s.clientUpdates <- ClientUpdate{"STATUS", payload}
	}
}

// RunRPCServer sets up and run a permanent JSON-RPC server.
func RunRPCServer(messageChan chan<- ClientUpdate, portrpc int) {
	server := rpc.NewServer()

	// Set up objects to handle remote calls
	sourcecontrol := new(SourceControl)
	sourcecontrol.clientUpdates = messageChan
	server.Register(sourcecontrol)
	go func() {
		ticker := time.Tick(2 * time.Second)
		for _ = range ticker {
			sourcecontrol.broadcastUpdate()
		}
	}()

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
