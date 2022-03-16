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

	"github.com/usnistgov/dastard/packets"
	"github.com/usnistgov/dastard/ringbuffer"
)

const packetAlign = 8192 // Packets go into the ring buffer at this stride (bytes)

func clearRings(nclear int) error {
	for cardnum := 0; cardnum < nclear; cardnum++ {
		ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
		ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
		ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
		if err != nil {
			return fmt.Errorf("Could not open ringbuffer: %s", err)
		}
		ring.Unlink() // in case it exists from before
	}
	return nil
}

// BahamaControl carries all the free parameters of the data generator
type BahamaControl struct {
	Nchan      int
	Ngroups    int
	Nsources   int
	Chan0      int // channel number of first channel
	chanGaps   int // how many channel numbers to skip between groups/rings
	ringsize   int // Bytes per ring
	udp        bool
	sinusoid   bool
	sawtooth   bool
	pulses     bool
	interleave bool
	stagger    bool
	noiselevel float64
	samplerate float64
	dropfrac   float64
}

// Report prints the Bahama configuration to the terminal.
func (control *BahamaControl) Report() {
	fmt.Println("Samples per second:       ", control.samplerate)
	fmt.Printf("Drop packets randomly:     %.2f%%\n", 100.0*control.dropfrac)
	if control.udp {
		fmt.Println("Generating UDP packets.")
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
		sources = append(sources, "pulses")
	}
	if control.sinusoid {
		sources = append(sources, "sinusoids")
	}
	fmt.Printf("Data will be the sum of these source types: %s.\n", strings.Join(sources, "+"))
	fmt.Println("Type Ctrl-C to stop generating Abaco-style data.")
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

func generateData(Nchan, firstchanOffset int, packetchan chan []byte, cancel chan os.Signal, control BahamaControl) error {
	// Data will play on infinite repeat with this many samples and this repeat period:
	const Nsamp = 40000 // This many samples before data repeats itself
	sampleRate := control.samplerate
	periodns := float64(Nsamp) / sampleRate * 1e9
	repeatTime := time.Duration(periodns+0.5) * time.Nanosecond // Repeat data with this period

	stride := 4000 / Nchan // We'll put this many samples into each packet
	valuesPerPacket := stride * Nchan
	if 2*valuesPerPacket > 8000 {
		return fmt.Errorf("Packet payload size %d exceeds 8000 bytes", 2*valuesPerPacket)
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
	TotalNvalues := Nchan * Nsamp // one rep has this many values
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
	const initSeqNum = 0
	packet := packets.NewPacket(version, sourceID, initSeqNum, firstchanOffset)

	// Raw data that will go into packets
	d := make([]int16, Nchan*Nsamp)
	for i := 0; i < Nchan; i++ {
		cnum := i + firstchanOffset
		offset := int16(cnum * 1000)
		for j := 0; j < Nsamp; j++ {
			d[i+Nchan*j] = offset
		}
		if control.sinusoid {
			freq := (float64(cnum+1) * 2 * math.Pi) / float64(Nsamp)
			amplitude := 8000.0
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(amplitude * math.Sin(freq*float64(j)))
			}
		}
		if control.sawtooth {
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(j % 5000)
			}
		}
		if control.pulses {
			amplitude := 5000.0 + 100.0*float64(cnum)
			scale := amplitude * 2.116
			for j := 0; j < Nsamp/4; j++ {
				pulsevalue := int16(scale * (math.Exp(-float64(j)/1200.0) - math.Exp(-float64(j)/300.0)))
				// Same pulse repeats 4x
				for k := 0; k < 4; k++ {
					d[i+Nchan*(j+k*(Nsamp/4))] += pulsevalue
				}
			}
		}
		if control.noiselevel > 0.0 {
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(randsource.NormFloat64() * control.noiselevel)
			}
		}

		// Abaco data only records the lowest N bits.
		// Wrap the 16-bit data properly into [0, (2^N)-1]
		// As of Jan 2021, this became N=16, so no "wrapping" needed.
		FractionBits := 16
		if FractionBits < 16 {
			for j := 0; j < Nsamp; j++ {
				raw := d[i+Nchan*j]
				d[i+Nchan*j] = raw % (1 << FractionBits)
			}
		}
	}

	dims := []int16{int16(Nchan)}
	timer := time.NewTicker(burstTime)
	timeCounter := uint64(0)
	for {
		for burstnum := 0; burstnum < nbursts; burstnum++ {
			select {
			case <-cancel:
				close(packetchan)
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
		return fmt.Errorf("Could not open ringbuffer %d: %s", cardnum, err)
	}
	ring.Unlink() // in case it exists from before
	if err = ring.Create(ringsize); err != nil {
		return fmt.Errorf("Failed RingBuffer.Create(%d): %s", ringsize, err)
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

func udpwriter(sourcenum int, packetchan chan []byte) error {
	hostname := fmt.Sprintf("localhost:%d", 4000+sourcenum)
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

func main() {
	maxRings := 4
	nchan := flag.Int("nchan", 4, "Number of channels per source, 4-512 allowed")
	ring := flag.Bool("ring", false, "Data into shared memory ring buffers instead of UDP packets (default false)")
	nsource := flag.Int("nsource", 1, "Number of sources (UDP clients or ring buffers), 1-4 allowed")
	chan0 := flag.Int("firstchan", 0, "Channel number of the first channel (default 0)")
	changaps := flag.Int("gaps", 0, "How many channel numbers to skip between groups (relevant only if ngroups>1 or nsource>1)")
	ngroups := flag.Int("ngroups", 1, "Number of channel groups per source, (1-Nchan/4) allowed")
	samplerate := flag.Float64("rate", 200000., "Samples per channel per second, 1000-500000")
	noiselevel := flag.Float64("noise", 0.0, "White noise level (<=0 means no noise)")
	usesawtooth := flag.Bool("saw", false, "Whether to add a sawtooth pattern")
	usesine := flag.Bool("sine", false, "Whether to add a sinusoidal pattern")
	usepulses := flag.Bool("pulse", false, "Whether to add pulse-like data")
	interleave := flag.Bool("interleave", false, "Whether to interleave channel groups' packets regularly")
	stagger := flag.Bool("stagger", false, "Whether to stagger channel groups' packets so each gets 'ahead' of the others")
	droppct := flag.Float64("droppct", 0.0, "Drop this percentage of packets")
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

	control := BahamaControl{Nchan: *nchan, Ngroups: *ngroups,
		Nsources: *nsource, udp: udp,
		Chan0: *chan0, chanGaps: *changaps, ringsize: ringsize,
		stagger: *stagger, interleave: *interleave,
		sawtooth: *usesawtooth, pulses: *usepulses,
		sinusoid: *usesine, noiselevel: *noiselevel,
		samplerate: *samplerate, dropfrac: (*droppct) / 100.0}

	control.Report()

	cancel := make(chan os.Signal)
	signal.Notify(cancel, os.Interrupt, syscall.SIGTERM)
	ch0 := control.Chan0
	for cardnum := 0; cardnum < control.Nsources; cardnum++ {
		packetchan := make(chan []byte)
		if control.udp {
			if err := udpwriter(cardnum, packetchan); err != nil {
				fmt.Printf("udpwriter(%d,...) failed: %v\n", cardnum, err)
				continue
			}
		} else {
			if err := ringwriter(cardnum, packetchan, ringsize); err != nil {
				fmt.Printf("ringwriter(%d,...) failed: %v\n", cardnum, err)
				continue
			}
		}

		stage1pchans := make([]chan []byte, control.Ngroups)
		chanPerGroup := (1 + (control.Nchan-1)/control.Ngroups)
		for i := 0; i < control.Nchan; i += chanPerGroup {
			nch := chanPerGroup
			if i*chanPerGroup+nch > control.Nchan {
				nch = control.Nchan - i
			}
			pchan := packetchan
			if control.interleave {
				pchan := make(chan []byte)
				stage1pchans = append(stage1pchans, pchan)
			}
			go func(nchan, chan0 int, pchan chan []byte) {
				if err := generateData(nchan, chan0, pchan, cancel, control); err != nil {
					fmt.Printf("generateData() failed: %v\n", err)
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
