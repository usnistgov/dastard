package dastard

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/signal"
	"path"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gonum.org/v1/gonum/mat"
)

// SourceControl is the sub-server that handles configuration and operation of
// the Dastard data sources.
// TODO: consider renaming -> DastardControl (5/11/18)
type SourceControl struct {
	simPulses *SimPulseSource
	triangle  *TriangleSource
	lancero   *LanceroSource
	// TODO: Add sources for ROACH, Abaco
	activeSource DataSource

	status        ServerStatus
	clientUpdates chan<- ClientUpdate
	totalData     Heartbeat
	heartbeats    chan Heartbeat
}

// NewSourceControl creates a new SourceControl object with correctly initialized
// contents.
func NewSourceControl() *SourceControl {
	sc := new(SourceControl)
	sc.heartbeats = make(chan Heartbeat)
	sc.simPulses = NewSimPulseSource()
	sc.simPulses.heartbeats = sc.heartbeats
	sc.triangle = NewTriangleSource()
	sc.triangle.heartbeats = sc.heartbeats
	if lan, err := NewLanceroSource(); err == nil {
		sc.lancero = lan
		sc.lancero.heartbeats = sc.heartbeats

	}
	sc.status.Ncol = make([]int, 0)
	sc.status.Nrow = make([]int, 0)
	return sc
}

// ServerStatus the status that SourceControl reports to clients.
type ServerStatus struct {
	Running    bool
	SourceName string
	Nchannels  int
	Nsamples   int
	Npresamp   int
	Ncol       []int
	Nrow       []int
	// TODO: maybe bytes/sec data rate...?
}

// Heartbeat is the info sent in the regular heartbeat to clients
type Heartbeat struct {
	Running bool
	Time    float64
	DataMB  float64
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
	s.clientUpdates <- ClientUpdate{"SIMPULSE", args}
	*reply = (err == nil)
	log.Printf("Result is okay=%t and state={%d chan, rate=%.3f}\n", *reply, s.simPulses.nchan, s.simPulses.sampleRate)
	return err
}

// ConfigureLanceroSource configures the lancero cards.
func (s *SourceControl) ConfigureLanceroSource(args *LanceroSourceConfig, reply *bool) error {
	log.Printf("ConfigureLanceroSource: mask 0x%4.4x  active cards: %v\n", args.FiberMask, args.ActiveCards)
	err := s.lancero.Configure(args)
	s.clientUpdates <- ClientUpdate{"LANCERO", args}
	*reply = (err == nil)
	log.Printf("Result is okay=%t and state={%d MHz clock, %d cards}\n", *reply, s.lancero.clockMhz, s.lancero.ncards)
	return err
}

// MixFractionObject is the RPC-usable structure for ConfigureMixFraction
type MixFractionObject struct {
	ProcessorIndex int
	MixFraction    float64
}

// ConfigureMixFraction sets the MixFraction for the channel associated with ProcessorIndex
// mix = fb + mixFraction*err/Nsamp
// This MixFractionObject contains mix fractions as reported by autotune, where error/Nsamp
// is used. Thus, we will internally store not MixFraction, but errorScale := MixFraction/Nsamp.
// NOTE: only supported by LanceroSource.
func (s *SourceControl) ConfigureMixFraction(mfo *MixFractionObject, reply *bool) error {
	if s.activeSource == nil {
		*reply = false
		return fmt.Errorf("No source is active")
	}
	err := s.activeSource.ConfigureMixFraction(mfo.ProcessorIndex, mfo.MixFraction)
	*reply = (err == nil)
	return err
}

// ConfigureTriggers configures the trigger state for 1 or more channels.
func (s *SourceControl) ConfigureTriggers(state *FullTriggerState, reply *bool) error {
	if s.activeSource == nil {
		return fmt.Errorf("No source is active")
	}
	err := s.activeSource.ChangeTriggerState(state)
	go s.broadcastTriggerState()
	*reply = (err == nil)
	return err
}

// ProjectorsBasisObject is the RPC-usable structure for ConfigureProjectorsBases
type ProjectorsBasisObject struct {
	ProcessorIndex   int
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
	if err := s.activeSource.ConfigureProjectorsBases(pbo.ProcessorIndex, projectors, basis); err != nil {
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
	s.broadcastStatus()
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
		s.activeSource = DataSource(s.simPulses)
		s.status.SourceName = "SimPulses"

	case "TRIANGLESOURCE":
		s.activeSource = DataSource(s.triangle)
		s.status.SourceName = "Triangles"

	case "LANCEROSOURCE":
		s.activeSource = DataSource(s.lancero)
		s.status.SourceName = "Lancero"

	// TODO: Add cases here for ROACH, ABACO, etc.

	default:
		return fmt.Errorf("Data Source \"%s\" is not recognized", *sourceName)
	}

	log.Printf("Starting data source named %s\n", *sourceName)
	s.status.Running = true
	if err := Start(s.activeSource); err != nil {
		s.status.Running = false
		s.activeSource = nil
		return err
	}
	s.status.Nchannels = s.activeSource.Nchan()
	if ls, ok := s.activeSource.(*LanceroSource); ok {
		s.status.Ncol = make([]int, ls.ncards)
		s.status.Nrow = make([]int, ls.ncards)
		for i, device := range ls.active {
			s.status.Ncol[i] = device.ncols
			s.status.Nrow[i] = device.nrows
		}
	} else {
		s.status.Ncol = make([]int, 0)
		s.status.Nrow = make([]int, 0)
	}
	s.broadcastStatus()
	s.broadcastTriggerState()
	s.broadcastChannelNames()
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
	s.status.Running = false
	s.activeSource = nil
	*reply = true
	// The following can't run without holding the s.mu lock, so it needs
	// to be launched in a separate goroutine.
	s.broadcastStatus()
	*reply = true
	return nil
}

// WriteControlConfig object to control start/stop/pause of data writing
// Path and FileType are ignored for any request other than Start
type WriteControlConfig struct {
	Request    string // "Start", "Stop", "Pause", or "Unpause"
	Path       string // write in a new directory under this path
	WriteLJH22 bool   // turn on one or more file formats
	WriteOFF   bool
	WriteLJH3  bool
}

// WriteControl requests start/stop/pause/unpause data writing
func (s *SourceControl) WriteControl(config *WriteControlConfig, reply *bool) error {
	*reply = true
	if s.activeSource == nil {
		return nil
	}
	err := s.activeSource.WriteControl(config)
	*reply = (err != nil)
	go s.broadcastWritingState()
	return err
}

// WriteComment writes the comment to comment.txt
func (s *SourceControl) WriteComment(comment *string, reply *bool) error {
	*reply = true
	if s.activeSource == nil || len(*comment) == 0 {
		return nil
	}
	ws := s.activeSource.ComputeWritingState()
	if ws.Active {
		dir := path.Dir(ws.Filename)
		commentName := path.Join(dir, "comment.txt")
		fp, err := os.Create(commentName)
		if err != nil {
			return err
		}
		defer fp.Close()
		fp.WriteString(*comment)
		// Always end the comment file with a newline.
		if !strings.HasSuffix(*comment, "\n") {
			fp.WriteString("\n")
		}
	}
	return nil
}

// CouplingStatus describes the status of FB / error coupling
type CouplingStatus int

// Specific allowed values for status of FB / error coupling
const (
	// FB and error aren't coupled
	NoCoupling CouplingStatus = iota + 1
	FBToErr                   // FB triggers cause secondary triggers in error channels
	ErrToFB                   // Error triggers cause secondary triggers in FB channels
)

// CoupleErrToFB turns on or off coupling of Error -> FB
func (s *SourceControl) CoupleErrToFB(couple *bool, reply *bool) error {
	if s.activeSource == nil {
		return fmt.Errorf("No source is active")
	}

	*reply = true
	c := NoCoupling
	if *couple {
		c = ErrToFB
	}
	err := s.activeSource.SetCoupling(c)
	s.clientUpdates <- ClientUpdate{"TRIGCOUPLING", c}
	if err != nil {
		*reply = false
	}
	return err
}

// CoupleFBToErr turns on or off coupling of FB -> Error
func (s *SourceControl) CoupleFBToErr(couple *bool, reply *bool) error {
	if s.activeSource == nil {
		return fmt.Errorf("No source is active")
	}

	*reply = true
	c := NoCoupling
	if *couple {
		c = FBToErr
	}
	err := s.activeSource.SetCoupling(c)
	s.clientUpdates <- ClientUpdate{"TRIGCOUPLING", c}
	if err != nil {
		*reply = false
	}
	return err
}

func (s *SourceControl) broadcastHeartbeat() {
	s.totalData.Running = s.status.Running
	s.clientUpdates <- ClientUpdate{"ALIVE", s.totalData}
	s.totalData.DataMB = 0
	s.totalData.Time = 0
}

func (s *SourceControl) broadcastStatus() {
	s.clientUpdates <- ClientUpdate{"STATUS", s.status}
}

func (s *SourceControl) broadcastWritingState() {
	if s.activeSource != nil && s.status.Running {
		state := s.activeSource.ComputeWritingState()
		s.clientUpdates <- ClientUpdate{"WRITING", state}
	}
}

func (s *SourceControl) broadcastTriggerState() {
	if s.activeSource != nil && s.status.Running {
		state := s.activeSource.ComputeFullTriggerState()
		log.Printf("TriggerState: %v\n", state)
		s.clientUpdates <- ClientUpdate{"TRIGGER", state}
	}
}

func (s *SourceControl) broadcastChannelNames() {
	if s.activeSource != nil && s.status.Running {
		configs := s.activeSource.ChannelNames()
		log.Printf("chanNames: %v\n", configs)
		s.clientUpdates <- ClientUpdate{"CHANNELNAMES", configs}
	}
}

// SendAllStatus causes a broadcast to clients containing all broadcastable status info
func (s *SourceControl) SendAllStatus(dummy *string, reply *bool) error {
	s.broadcastStatus()
	s.clientUpdates <- ClientUpdate{"SENDALL", 0}
	return nil
}

// RunRPCServer sets up and run a permanent JSON-RPC server.
// if block, it will block until Ctrl-C and gracefully shut down
func RunRPCServer(portrpc int, block bool) {

	// Set up objects to handle remote calls
	sourceControl := NewSourceControl()
	defer sourceControl.lancero.Delete()
	sourceControl.clientUpdates = clientMessageChan

	// Load stored settings, and transfer saved configuration
	// from Viper to relevant objects.
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
	var lsc LanceroSourceConfig
	err = viper.UnmarshalKey("lancero", &lsc)
	if err == nil {
		sourceControl.ConfigureLanceroSource(&lsc, &okay)
	}
	err = viper.UnmarshalKey("status", &sourceControl.status)
	sourceControl.status.Running = false
	if err == nil {
		sourceControl.broadcastStatus()
	}
	var ws WritingState
	err = viper.UnmarshalKey("writing", &ws)
	if err == nil {
		wsSend := WritingState{BasePath: ws.BasePath} // only send the BasePath to clients
		// other info like Active: true could be wrong, and is not useful
		sourceControl.clientUpdates <- ClientUpdate{"WRITING", wsSend}
	}

	// Regularly broadcast a "heartbeat" containing data rate to all clients
	go func() {
		ticker := time.Tick(2 * time.Second)
		for {
			select {
			case <-ticker:
				go sourceControl.broadcastHeartbeat()
			case h := <-sourceControl.heartbeats:
				sourceControl.totalData.DataMB += h.DataMB
				sourceControl.totalData.Time += h.Time
			}
		}
	}()

	// Now launch the connection handler and accept connections.

	go func() {
		server := rpc.NewServer()
		if err := server.Register(sourceControl); err != nil {
			log.Fatal(err)
		}
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
				go func() { // this is equivalent to ServeCodec, except all requests are
					// handled SYNCHRONOUSLY, so sourceControl doesn't need a lock
					codec := jsonrpc.NewServerCodec(conn)
					for {
						err := server.ServeRequest(codec)
						if err != nil {
							log.Printf("server stopped: %v", err)
							break
						}
					}
				}()
			}
		}
	}()

	if block {
		// Finally, handle ctrl-C gracefully
		interruptCatcher := make(chan os.Signal, 1)
		signal.Notify(interruptCatcher, os.Interrupt)
		<-interruptCatcher
		dummy := "dummy"
		sourceControl.Stop(&dummy, &okay)
	}
}
