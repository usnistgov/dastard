package dastard

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gonum.org/v1/gonum/mat"
)

// SourceControl is the sub-server that handles configuration and operation of
// the Dastard data sources.
// TODO: consider renaming -> DastardControl (5/11/18)
type SourceControl struct {
	simPulses SimPulseSource
	triangle  TriangleSource
	// TODO: Add sources for Lancero, ROACH, Abaco
	activeSource DataSource

	status        ServerStatus
	clientUpdates chan<- ClientUpdate
}

// ServerStatus the status that SourceControl reports to clients.
type ServerStatus struct {
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
	log.Printf("ConfigureTriangleSource: %d chan, rate=%.3f\n", args.Nchan, args.SampleRate)
	err := s.triangle.Configure(args)
	s.clientUpdates <- ClientUpdate{"TRIANGLE", args}
	*reply = (err == nil)
	log.Printf("Result is okay=%t and state={%d chan, rate=%.3f}\n", *reply, s.triangle.nchan, s.triangle.sampleRate)
	return err
}

// ConfigureSimPulseSource configures the source of simulated pulses.
func (s *SourceControl) ConfigureSimPulseSource(args *SimPulseSourceConfig, reply *bool) error {
	log.Printf("ConfigureSimPulseSource: %d chan, rate=%.3f\n", args.Nchan, args.SampleRate)
	err := s.simPulses.Configure(args)
	s.clientUpdates <- ClientUpdate{"SIMPULSES", args}
	*reply = (err == nil)
	log.Printf("Result is okay=%t and state={%d chan, rate=%.3f}\n", *reply, s.simPulses.nchan, s.simPulses.sampleRate)
	return err
}

// ProjectorsBasisObject is the RPC-usable structure for
type ProjectorsBasisObject struct {
	ProcessorInd     int
	ProjectorsBase64 string
	BasisBase64      string
}

// ConfigureProjectorsBasis takes ProjectorsBase64 which must a base64 encoded string with binary data matching that from mat.Dense.MarshalBinary
func (s *SourceControl) ConfigureProjectorsBasis(pbo *ProjectorsBasisObject, reply *bool) error {
	if s.activeSource == nil {
		return fmt.Errorf("No source is active")
	}
	projectorsBytes, err := base64.StdEncoding.DecodeString(pbo.ProjectorsBase64)
	if err != nil {
		return err
	}
	basisBytes, err := base64.StdEncoding.DecodeString(pbo.BasisBase64)
	if err != nil {
		return err
	}
	var projectors, basis mat.Dense
	if err := projectors.UnmarshalBinary(projectorsBytes); err != nil {
		return err
	}
	if err := basis.UnmarshalBinary(basisBytes); err != nil {
		return err
	}
	if err := s.activeSource.ConfigureProjectorsBases(pbo.ProcessorInd, projectors, basis); err != nil {
		return err
	}
	*reply = true
	return nil
}

// SizeObject is the RPC-usable structure for ConfigurePulseLengths to change pulse record sizes.
type SizeObject struct {
	Nsamp int
	Npre  int
}

// ConfigurePulseLengths is the RPC-callable service to change pulse record sizes.
func (s *SourceControl) ConfigurePulseLengths(sizes SizeObject, reply *bool) error {
	log.Printf("ConfigurePulseLengths: %d samples (%d pre)\n", sizes.Nsamp, sizes.Npre)
	if s.activeSource == nil {
		return fmt.Errorf("No source is active")
	}
	err := s.activeSource.ConfigurePulseLengths(sizes.Nsamp, sizes.Npre)
	*reply = (err == nil)
	s.status.Npresamp = sizes.Npre
	s.status.Nsamples = sizes.Nsamp
	log.Printf("Result is okay=%t\n", *reply)
	return err
}

// Start will identify the source given by sourceName and Sample then Start it.
func (s *SourceControl) Start(sourceName *string, reply *bool) error {
	if s.activeSource != nil {
		return fmt.Errorf("activeSource is not nil, want nil (you should call Stop)")
	}
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

	log.Printf("Starting data source named %s\n", *sourceName)
	go func() {
		err := Start(s.activeSource)
		if err == nil {
			s.status.Running = true
			s.status.Nchannels = s.activeSource.Nchan()
			s.broadcastUpdate()
			s.clientUpdates <- ClientUpdate{"TRIANGLE", sourceName}
			s.broadcastTriggerState()
		}
	}()
	*reply = true
	return nil
}

// Stop stops the running data source, if any
func (s *SourceControl) Stop(dummy *string, reply *bool) error {
	if s.activeSource == nil {
		return fmt.Errorf("No source is active")
	}
	log.Printf("Stopping data source\n")
	s.activeSource.Stop()
	s.activeSource = nil

	s.status.Running = false
	s.status.SourceName = ""
	s.status.Nchannels = 0
	s.broadcastUpdate()
	*reply = true
	return nil
}

func (s *SourceControl) broadcastUpdate() {
	s.clientUpdates <- ClientUpdate{"STATUS", s.status}
}

func (s *SourceControl) broadcastTriggerState() {
	if s.activeSource != nil && s.status.Running {
		configs := s.activeSource.ComputeFullTriggerState()
		log.Printf("configs: %v\n", configs)
		s.clientUpdates <- ClientUpdate{"TRIGGER", configs}
	}
}

// SendAllStatus causes a broadcast to clients containing all broadcastable status info
func (s *SourceControl) SendAllStatus(dummy *string, reply *bool) error {
	s.broadcastUpdate()
	s.clientUpdates <- ClientUpdate{"SENDALL", 0}
	return nil
}

// RunRPCServer sets up and run a permanent JSON-RPC server.
func RunRPCServer(messageChan chan<- ClientUpdate, portrpc int) {

	// Set up objects to handle remote calls
	sourceControl := new(SourceControl)
	sourceControl.clientUpdates = messageChan

	// Load stored settings
	var okay bool
	var spc SimPulseSourceConfig
	log.Printf("Dastard is using config file %s\n", viper.ConfigFileUsed())
	err := viper.UnmarshalKey("simpulse", &spc)
	if err == nil {
		sourceControl.ConfigureSimPulseSource(&spc, &okay)
	}
	var tsc TriangleSourceConfig
	err = viper.UnmarshalKey("triangle", &tsc)
	if err == nil {
		sourceControl.ConfigureTriangleSource(&tsc, &okay)
	}

	go func() {
		ticker := time.Tick(2 * time.Second)
		for _ = range ticker {
			sourceControl.broadcastUpdate()
		}
	}()

	// Transfer saved configuration from Viper to relevant objects.

	// Now launch the connection handler and accept connections.
	server := rpc.NewServer()
	server.Register(sourceControl)
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
