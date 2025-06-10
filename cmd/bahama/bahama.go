package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"sync"

	"github.com/usnistgov/dastard/packets"
	"github.com/usnistgov/dastard/ringbuffer"
	"log"
	"net/rpc"
	"net/rpc/jsonrpc"
)

const packetAlign = 8192 // Packets go into the ring buffer at this stride (bytes)

func clearRings(nclear int) error {
	for cardnum := 0; cardnum < nclear; cardnum++ {
		ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
		ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
		ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
		if err != nil {
			return fmt.Errorf("could not open ringbuffer: %s", err)
		}
		ring.Unlink() // in case it exists from before
	}
	return nil
}

// BahamaControl carries all the free parameters of the data generator
type BahamaControl struct {
	Nchan      int
	Nsamp 	   int
	Ngroups    int
	Nsources   int
	Chan0      int // channel number of first channel
	chanGaps   int // how many channel numbers to skip between groups/rings
	ringsize   int // Bytes per ring
	port       int
	udp        bool
	sinusoid   bool
	sawtooth   bool
	pulses     bool
	crosstalk  bool
	interleave bool
	stagger    bool
	noiselevel float64
	samplerate float64
	dropfrac   float64
	host       string
	cancel     chan os.Signal
	Pulse      PulseParams
	Sawtooth   SawtoothParams
	Sinusoid   SinusoidParams
	generatedData []int16
	generatedDataShort []int16
	dataMutex     sync.RWMutex
	publisherRunning bool
}

type PulseParams struct {
    Amplitudes []float64
    Width      int
}

type SawtoothParams struct {
    Amplitude float64
    Period    int
}

type SinusoidParams struct {
    Amplitude float64
    Frequency float64
    Phase     float64
}

// Report prints the Bahama configuration to the terminal.
func (control *BahamaControl) Report() {
	fmt.Println("Samples per second:       ", control.samplerate)
	fmt.Println("Nsamp:                    ", control.Nsamp)
	fmt.Printf("Drop packets randomly:     %.2f%%\n", 100.0*control.dropfrac)
	if control.udp {
		fmt.Println("Generating UDP packets on port ", control.port)
		fmt.Println("Number of UDP sources:    ", control.Nsources)
		fmt.Println("Channels per UDP source:  ", control.Nchan)
		fmt.Println("Channel groups per UDP:   ", control.Ngroups)
	} else {
		fmt.Println("Putting packets into ring buffers.")
		fmt.Println("Number of ring buffers:   ", control.Nsources)
		fmt.Println("Size of each ring:        ", control.ringsize)
		fmt.Println("Channels per ring:        ", control.Nchan)
		fmt.Println("Channel groups per ring:  ", control.Ngroups)
	}
	fmt.Println("Channel # of 1st chan:    ", control.Chan0)
	if control.Ngroups > 1 || control.Nsources > 1 {
		fmt.Println("Skip # btwn groups/sources: ", control.chanGaps)
	}
	if control.Ngroups > 1 {
		if control.stagger {
			control.interleave = true
		}
	} else {
		if control.stagger || control.interleave {
			fmt.Println("Warning: -stagger and -interleave arguments are ignored with only 1 channel group")
		}
		control.stagger = false
		control.interleave = false
	}
	if control.stagger {
		fmt.Println("  Packets from channel groups are staggered so each group is sometimes ahead of the others")
	} else if control.interleave {
		fmt.Println("  Packets from channel groups are interleaved regularly")
	}

	if !(control.noiselevel > 0.0 || control.sawtooth || control.pulses || control.sinusoid) {
		control.sawtooth = true
	}
	var sources []string
	if control.noiselevel > 0.0 {
		sources = append(sources, "noise")
	}
	if control.sawtooth {
		sources = append(sources, "sawtooth")
	}
	if control.pulses {
		if control.crosstalk {
			sources = append(sources, "pulses with crosstalk")
		} else {
			sources = append(sources, "pulses")
		}
	}
	if control.sinusoid {
		sources = append(sources, "sinusoids")
	}
	fmt.Printf("Data will be the sum of these source types: %s.\n", strings.Join(sources, "+"))
	fmt.Println("Type Ctrl-C to stop generating Abaco-style data.")

	if control.pulses {
        fmt.Printf("Pulse amplitudes: %v\n", control.Pulse.Amplitudes)
        fmt.Printf("Pulse width: %d\n", control.Pulse.Width)
    }
	if control.sawtooth {
        fmt.Printf("Sawtooth amplitude: %.2f\n", control.Sawtooth.Amplitude)
        fmt.Printf("Sawtooth period: %d\n", control.Sawtooth.Period)
    }
    if control.sinusoid {
        fmt.Printf("Sinusoid amplitude: %.2f\n", control.Sinusoid.Amplitude)
        fmt.Printf("Sinusoid frequency: %.2f\n", control.Sinusoid.Frequency)
        fmt.Printf("Sinusoid phase: %.2f\n", control.Sinusoid.Phase)
    }
}

// interleavePackets takes packets from the N channels `inchans` and puts them onto `outchan` in a
// specific order to test Dastard's handling of various packet orderings
func interleavePackets(outchan chan []byte, inchans []chan []byte, stagger bool) {
	// If stagger is false, then packets will go onto `outchan` in the order ABCABCABC... if there are 3 groups.
	// If stagger is true, then they will start out staggered: ABBCCCAAABBBCCCAAABBBCCC if there are 3 groups.
	// The latter ensures that each group is "ahead" of all others at some point in the sequence.
	// In the above A, B, C represent not a single packet from the channel groups, but `bucketsize` packets.
	const bucketsize = 4
	latersize := bucketsize
	if stagger {
		latersize *= len(inchans)
	}
	sendsize := make([]int, len(inchans))
	for i := range inchans {
		if stagger {
			sendsize[i] = (i + 1) * bucketsize
		} else {
			sendsize[i] = bucketsize
		}
	}

	someopen := true
	for someopen {
		someopen = false
		for i, ic := range inchans {
			for j := 0; j < sendsize[i]; j++ {
				p, ok := <-ic
				if ok {
					someopen = true
					outchan <- p
				}
			}
			sendsize[i] = latersize
		}
	}
	close(outchan)
}

func generateData(control *BahamaControl) error {
	// Data will play on infinite repeat with this many samples and this repeat period:
	// This many samples before data repeats itself
	// fmt.Printf("Breaking data into %d bursts with %d values each, burst time %v\n", nbursts, BurstNvalues, burstTime)

	Nchan := (1 + (control.Nchan-1)/control.Ngroups)
	ShortNchan := control.Nchan % control.Ngroups
	Nsamp := control.Nsamp
	randsource := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Raw data that will go into packets
	d := make([]int16, Nchan * Nsamp)
	ds := make([]int16, ShortNchan * Nsamp)
	for i := 0; i < Nchan; i++ {
		cnum := i
		offset := int16(cnum * 1000)
		for j := 0; j < Nsamp; j++ {
			d[i+Nchan*j] = offset
			if i < ShortNchan {
				ds[i + ShortNchan*j] = offset
			}
		}
		if control.sinusoid {
			freq := control.Sinusoid.Frequency * 2 * math.Pi / float64(Nsamp)
			for j := 0; j < Nsamp; j++ {
				sinvalue := int16(control.Sinusoid.Amplitude * math.Sin(freq*float64(j) + control.Sinusoid.Phase * float64(i)))
				d[i+Nchan*j] += sinvalue
				if i < ShortNchan {
					ds[i + ShortNchan*j] += sinvalue
				}			}
		}
		if control.sawtooth {
			for j := 0; j < Nsamp; j++ {
				sawvalue := int16(control.Sawtooth.Amplitude * float64(j%control.Sawtooth.Period) / float64(control.Sawtooth.Period))
				d[i+Nchan*j] += sawvalue
				if i < ShortNchan {
					ds[i + ShortNchan*j] += sawvalue
				}
			}
		}
		if control.pulses {
			amplitudes := control.Pulse.Amplitudes
			width := control.Pulse.Width
			numAmplitudes := len(amplitudes)
			for j := 0; j < width; j++ {
				for k := 0; k < int(Nsamp/width); k++ {
					amplitude := amplitudes[k%numAmplitudes]
					if control.crosstalk && i%2 == 1 {
						amplitude *= 0.02
					}
					scale := amplitude * 2.598
					pulsevalue := int16(scale * (math.Exp(-30.0*float64(j)/(3.0*float64(width))) - math.Exp(-30.0*float64(j)/float64(width))))
					d[i+Nchan*(j+k*width)] += pulsevalue
					if i < ShortNchan {
						ds[i + ShortNchan*(j + k*width)] += pulsevalue
					}
				}
			}
		}
		if control.noiselevel > 0.0 {
			for j := 0; j < Nsamp; j++ {
				noisevalue := int16(randsource.NormFloat64() * control.noiselevel)
				d[i+Nchan*j] += noisevalue
				if i < ShortNchan {
					ds[i + ShortNchan*j] += noisevalue
				}
			}
		}
		// Print 100 values of d, but only every Nchan'th value
		fmt.Println("Sample values for channel", i)
		for j := 0; j < 100*Nchan && j+i < len(d); j += Nchan {
			fmt.Printf("%d ", d[j+i])
			if (j/Nchan+1)%10 == 0 {
				fmt.Println()
			}
		}
		
		fmt.Println()
		// Abaco data only records the lowest N bits.
		// Wrap the 16-bit data properly into [0, (2^N)-1]
		// As of Jan 2021, this became N=16, so no "wrapping" needed.
	}
	control.dataMutex.Lock()
    control.generatedData = d
	control.generatedDataShort = ds
    control.dataMutex.Unlock()
	return nil
}

func regenerateData(control *BahamaControl) error {
    if err := generateData(control); err != nil {
        return err
    }
    fmt.Println("Data regenerated successfully")
    return nil
}

func (control *BahamaControl) RegenerateData(dummy *struct{}, reply *bool) error {
    err := regenerateData(control)
	*reply = (err == nil)
	return err
}

func publishData(Nchan, firstchanOffset int, packetchan chan []byte, cancel chan os.Signal, control *BahamaControl) error {
	// Data will play on infinite repeat with this many samples and this repeat period:
	control.publisherRunning = true
    defer func() { control.publisherRunning = false }()

	BaseNchan := (1 + (control.Nchan-1)/control.Ngroups)
	Nsamp := control.Nsamp
	sampleRate := control.samplerate
	periodns := float64(Nsamp) / sampleRate * 1e9
	repeatTime := time.Duration(periodns+0.5) * time.Nanosecond // Repeat data with this period

	stride := 4000 / Nchan // We'll put this many samples into each packet
	valuesPerPacket := stride * Nchan
	if 2*valuesPerPacket > 8000 {
		return fmt.Errorf("packet payload size %d exceeds 8000 bytes", 2*valuesPerPacket)
	}
	const counterRate = 1e8 // counts per second
	countsPerSample := uint64(counterRate/sampleRate + 0.5)

	// Unfortunately, repetition after Nsamp values might take longer than we want at low sampleRate.
	// Strategy: break one rep of data into N bursts, which will be sent ~10 ms apart.
	nbursts := int(repeatTime+5*time.Millisecond) / int(10*time.Millisecond)
	if nbursts <= 0 {
		nbursts = 1
	}
	burstTime := repeatTime / time.Duration(nbursts)
	TotalNvalues := Nsamp * Nchan // one rep has this many values
	BurstNvalues := TotalNvalues / nbursts
	BurstNvalues -= BurstNvalues % valuesPerPacket // bursts should have integer number of packets
	for BurstNvalues*nbursts < TotalNvalues {      // last burst should be shorter, not longer than the others.
		BurstNvalues += valuesPerPacket
	}
	// fmt.Printf("Breaking data into %d bursts with %d values each, burst time %v\n", nbursts, BurstNvalues, burstTime)

	randsource := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Raw packets
	const version = 10
	const sourceID = 20
	initSeqNum := uint32(randsource.Intn(10000))
	packet := packets.NewPacket(version, sourceID, initSeqNum, firstchanOffset)


	dims := []int16{int16(Nchan)}
	timer := time.NewTicker(burstTime)
	timeCounter := uint64(0)


	for {
		var d []int16
		control.dataMutex.RLock()
	
		if Nchan < BaseNchan {
			d = control.generatedDataShort
		} else {
			d = control.generatedData
		}
	
		control.dataMutex.RUnlock()

		for burstnum := 0; burstnum < nbursts; burstnum++ {
			select {
			case <-cancel:
				return nil
			case <-timer.C:
				lastvalue := (burstnum + 1) * BurstNvalues
				if TotalNvalues < lastvalue {
					lastvalue = TotalNvalues
				}

				for firstsamp := burstnum * BurstNvalues; firstsamp < lastvalue; firstsamp += valuesPerPacket {
					// Must be careful: the last iteration might have fewer samples than the others.
					lastsamp := firstsamp + valuesPerPacket
					if lastsamp > TotalNvalues {
						lastsamp = TotalNvalues
					}
					// Do we generate and send a packet, or drop it?
					if control.dropfrac == 0.0 || control.dropfrac < randsource.Float64() {
						packet.NewData(d[firstsamp:lastsamp], dims)
						ts := packets.MakeTimestamp(uint16(timeCounter>>32), uint32(timeCounter), counterRate)
						packet.SetTimestamp(ts)
						packetchan <- packet.Bytes()
					} else {
						// If dropping a packet, we still need to increment the serial number
						packet.NewData(d[0:0], dims)
					}
					timeCounter += countsPerSample * uint64((lastsamp-firstsamp)/Nchan)
				}
			}
		}
	}
}


func ringwriter(cardnum int, packetchan chan []byte, ringsize int) error {
	ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
	ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		return fmt.Errorf("could not open ringbuffer %d: %s", cardnum, err)
	}
	ring.Unlink() // in case it exists from before
	if err = ring.Create(ringsize); err != nil {
		return fmt.Errorf("failed RingBuffer.Create(%d): %s", ringsize, err)
	}
	fmt.Printf("Generating data in shm:%s\n", ringname)

	go func() {
		defer ring.Unlink() // so it won't exist after
		empty := make([]byte, packetAlign)
		for {
			b, ok := <-packetchan
			if !ok {
				return
			}
			if len(b) > packetAlign {
				b = b[:packetAlign]
			} else if len(b) < packetAlign {
				b = append(b, empty[:packetAlign-len(b)]...)
			}
			if ring.BytesWriteable() >= len(b) {
				ring.Write(b)
			}
		}
	}()
	return nil
}

// udpwriter is called once per data source (i.e., per UDP port producing data)
func udpwriter(portnum int, packetchan chan []byte, host string) error {
	hostname := fmt.Sprintf("%s:%d", host, portnum)
	addr, err := net.ResolveUDPAddr("udp", hostname)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}

	go func() {
		defer conn.Close() // so it won't exist after
		for {
			b, ok := <-packetchan
			if !ok {
				return
			}
			conn.Write(b)
		}
	}()
	return nil
}

func coerceInt(f *int, minval, maxval int) {
	if *f < minval {
		*f = minval
	}
	if *f > maxval {
		*f = maxval
	}
}

func coerceFloat(f *float64, minval, maxval float64) {
	if *f < minval {
		*f = minval
	}
	if *f > maxval {
		*f = maxval
	}
}

// BahamaControlUpdate carries the parameters that can be updated during runtime
type BahamaControlUpdate struct {
    Noiselevel float64
    Samplerate float64
    Dropfrac   float64
	NSamples   int
    Crosstalk  bool
    Sawtooth   bool
    Pulse     bool
    Sinusoid   bool
}

// ConfigureBahama updates the BahamaControl fields with the provided values
func (control *BahamaControl) ConfigureBahama(args *BahamaControlUpdate, reply *bool) error {
	if args.NSamples > 0 {
		control.Nsamp = args.NSamples
	}

    control.noiselevel = args.Noiselevel
    if args.Samplerate > 0 {
        control.samplerate = args.Samplerate
    }
    if args.Dropfrac >= 0 && args.Dropfrac <= 1 {
        control.dropfrac = args.Dropfrac
    }
    control.crosstalk = args.Crosstalk
    control.sawtooth = args.Sawtooth
    control.pulses = args.Pulse
    control.sinusoid = args.Sinusoid

    // Ensure at least one data source is active
    if !(control.noiselevel > 0 || control.sawtooth || control.pulses || control.sinusoid) {
        control.sawtooth = true
    }
    control.Report()
    *reply = true
    return nil
}

// ConfigurePulses updates the pulse parameters
func (control *BahamaControl) ConfigurePulses(args *PulseParams, reply *bool) error {
    control.Pulse = *args
    control.pulses = true
    control.Report()
    *reply = true
    return nil
}

// ConfigureSawtooth updates the sawtooth parameters
func (control *BahamaControl) ConfigureSawtooth(args *SawtoothParams, reply *bool) error {
    control.Sawtooth = *args
    control.sawtooth = true
    control.Report()
    *reply = true
    return nil
}

// ConfigureSinusoid updates the sinusoid parameters
func (control *BahamaControl) ConfigureSinusoid(args *SinusoidParams, reply *bool) error {
    control.Sinusoid = *args
    control.sinusoid = true
    control.Report()
    *reply = true
    return nil
}

// Start runs generateAndPublishData
func (control *BahamaControl) Start(dummy *string, reply *bool) error {
    if control.cancel != nil {
        return fmt.Errorf("data generation is already running")
    }
    control.cancel = make(chan os.Signal, 1)
    go generateAndPublishData(control, control.cancel)
    *reply = true
    return nil
}

// Stop stops data generation and publishing
func (control *BahamaControl) Stop(dummy *string, reply *bool) error {
    if control.cancel == nil {
        return fmt.Errorf("no data generation is running")
    }
    close(control.cancel)
    control.cancel = nil
    *reply = true
    return nil
}

// RunRPCServer sets up and runs a permanent JSON-RPC server for Bahama
func RunRPCServer(portrpc int, control *BahamaControl) {
    server := rpc.NewServer()
    if err := server.Register(control); err != nil {
        log.Fatal("Error registering RPC server:", err)
    }

    server.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
    port := fmt.Sprintf(":%d", portrpc)
    listener, err := net.Listen("tcp", port)
    if err != nil {
        log.Fatal("Listen error:", err)
    }

    log.Printf("RPC server listening on port %d\n", portrpc)

    for {
        if conn, err := listener.Accept(); err != nil {
            log.Fatal("Accept error:", err)
        } else {
            log.Printf("New client connection established\n")
            go func() {
                codec := jsonrpc.NewServerCodec(conn)
                for {
                    err := server.ServeRequest(codec)
                    if err != nil {
                        log.Printf("Error serving request: %v", err)
                        break
                    }
                    log.Printf("Served a request successfully")
                }
                log.Printf("Client connection closed")
            }()
        }
    }
}

func main() {
	maxRings := 4
	nchan := flag.Int("nchan", 4, "Number of channels per source, 4-512 allowed")
	nsamp := flag.Int("nsamp", 10000, "Number of samples per channel")
	ring := flag.Bool("ring", false, "Data into shared memory ring buffers instead of UDP packets (default false)")
	nsource := flag.Int("nsource", 1, "Number of sources (UDP clients or ring buffers), 1-4 allowed")
	chan0 := flag.Int("firstchan", 0, "Channel number of the first channel (default 0)")
	changaps := flag.Int("gaps", 0, "How many channel numbers to skip between groups (relevant only if ngroups>1 or nsource>1)")
	ngroups := flag.Int("ngroups", 1, "Number of channel groups per source, (1-Nchan/4) allowed")
	port := flag.Int("port", 4000, "UDP port to produce data")
	samplerate := flag.Float64("rate", 200000., "Samples per channel per second, 1000-500000")
	noiselevel := flag.Float64("noise", 0.0, "White noise level (<=0 means no noise)")
	usesawtooth := flag.Bool("saw", false, "Whether to add a sawtooth pattern")
	usesine := flag.Bool("sine", false, "Whether to add a sinusoidal pattern")
	usepulses := flag.Bool("pulse", false, "Whether to add pulse-like data")
	crosstalk := flag.Bool("crosstalk", false, "Whether to have only crosstalk to odd-numbered channels (implies --pulse)")
	interleave := flag.Bool("interleave", false, "Whether to interleave channel groups' packets regularly")
	stagger := flag.Bool("stagger", false, "Whether to stagger channel groups' packets so each gets 'ahead' of the others")
	droppct := flag.Float64("droppct", 0.0, "Drop this percentage of packets")
	host := flag.String("host", "0.0.0.0", "Hostname or IP address to bind to (use 0.0.0.0 for all interfaces)")
	portrpc := flag.Int("rpc", 5555, "RPC server port")
	pulseAmplitude := flag.Float64("pulseamp", 1000.0, "Amplitude of pulses (can be overridden by RPC)")
	pulseWidth := flag.Int("pulsewidth", 100, "Width of pulses")
	sawtoothAmplitude := flag.Float64("sawamp", 1000.0, "Amplitude of sawtooth wave")
	sawtoothPeriod := flag.Int("sawperiod", 1000, "Period of sawtooth wave")
	sinusoidAmplitude := flag.Float64("sineamp", 1000.0, "Amplitude of sinusoid wave")
	sinusoidFrequency := flag.Float64("sinefreq", 1.0, "Frequency of sinusoid wave")
	sinusoidPhase := flag.Float64("sinephase", 0.0, "Phase of sinusoid wave")
	flag.Usage = func() {
		fmt.Println("BAHAMA, the Basic Abaco Hardware Artificial Message Assembler")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		fmt.Println("If none of {noise, saw, sine, pulse} are given, saw will be used.")
	}
	flag.Parse()

	clearRings(maxRings)
	coerceInt(nchan, 4, 512)
	coerceInt(nsource, 1, 4)
	coerceInt(ngroups, 1, *nchan/4)
	coerceInt(changaps, 0, math.MaxInt64)
	coerceFloat(samplerate, 1000, 500000)
	coerceFloat(droppct, 0, 100)

	// Make sure there is some nonzero output
	if *noiselevel <= 0 {
		*noiselevel = 0.0
		if !(*usesawtooth || *usesine || *usepulses) {
			*usesawtooth = true
		}
	}

	// Ensure nsamp is greater than 0
	if *nsamp <= 0 {
		*nsamp = 10000 // Set a default value if nsamp is not positive
		fmt.Println("Warning: nsamp must be positive. Setting to default value of 10000.")
	}

	// Compute the size of each ring buffer. Let it be big enough to hold 0.5 seconds of data
	// or 500 packets, and be a multiple of packetAlign.
	ringlasts := 0.5 // seconds
	byterate := 2.0 * float64(*nchan) * (*samplerate)
	ringsize := int(byterate*ringlasts) + packetAlign
	ringsize -= ringsize % packetAlign
	if ringsize < 500*packetAlign {
		ringsize = 500 * packetAlign
	}
	udp := !(*ring)
	if udp {
		ringsize = 0
	}
	if *crosstalk {
		*usepulses = true
	}

	control := BahamaControl{Nchan: *nchan, Nsamp: *nsamp, Ngroups: *ngroups,
		Nsources: *nsource, udp: udp, port: *port,
		Chan0: *chan0, chanGaps: *changaps, ringsize: ringsize,
		stagger: *stagger, interleave: *interleave,
		sawtooth: *usesawtooth, pulses: *usepulses, crosstalk: *crosstalk,
		sinusoid: *usesine, noiselevel: *noiselevel,
		samplerate: *samplerate, dropfrac: (*droppct) / 100.0, host: *host, cancel: nil,
		Pulse: PulseParams{
			Amplitudes: []float64{*pulseAmplitude},
			Width:      *pulseWidth,
		},
		Sawtooth: SawtoothParams{
			Amplitude: *sawtoothAmplitude,
			Period:    *sawtoothPeriod,
		},
		Sinusoid: SinusoidParams{
			Amplitude: *sinusoidAmplitude,
			Frequency: *sinusoidFrequency,
			Phase:     *sinusoidPhase,
		},
	}

	control.Report()

	// Start the RPC server
	go RunRPCServer(*portrpc, &control)

	// Wait for interrupt signal
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)
	<-interruptChan

	fmt.Println("\nReceived interrupt signal. Shutting down...")
}

func generateAndPublishData(control *BahamaControl, cancel chan os.Signal) {
	signal.Notify(cancel, os.Interrupt, syscall.SIGTERM)
	ch0 := control.Chan0

	if err := generateData(control); err != nil {
        fmt.Printf("Initial data generation failed: %v\n", err)
        return
    }
	for cardnum := 0; cardnum < control.Nsources; cardnum++ {
		packetchan := make(chan []byte)
		defer close(packetchan)
		if control.udp {
			portnum := (control.port) + cardnum
			if err := udpwriter(portnum, packetchan, control.host); err != nil {
				fmt.Printf("udpwriter(%d, %s, ...) failed: %v\n", portnum, control.host, err)
				continue
			}
		} else {
			if err := ringwriter(cardnum, packetchan, control.ringsize); err != nil {
				fmt.Printf("ringwriter(%d,...) failed: %v\n", cardnum, err)
				continue
			}
		}

		stage1pchans := make([]chan []byte, control.Ngroups)
		chanPerGroup := (1 + (control.Nchan-1)/control.Ngroups)
		for i := 0; i < control.Nchan; i += chanPerGroup {
			nch := chanPerGroup
			if i+nch > control.Nchan {
				nch = control.Nchan - i
			}
			pchan := packetchan
			if control.interleave {
				pchan := make(chan []byte)
				stage1pchans = append(stage1pchans, pchan)
			}
			go func(nchan, chan0 int, pchan chan []byte) {
				if err := publishData(nchan, chan0, pchan, cancel, control); err != nil {
					fmt.Printf("publishData() failed: %v\n", err)
				}
			}(nch, ch0, pchan)
			ch0 += nch + control.chanGaps
		}
		if control.interleave {
			go interleavePackets(packetchan, stage1pchans, control.stagger)
		}
	}
	<-cancel
}
