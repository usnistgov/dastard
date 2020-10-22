package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
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
	Nrings     int
	Chan0      int // channel number of first channel
	chanGaps   int // how many channel numbers to skip between groups/rings
	ringsize   int // Bytes per ring
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
	fmt.Println("Number of ring buffers:   ", control.Nrings)
	fmt.Println("Size of each ring:        ", control.ringsize)
	fmt.Println("Channels per ring:        ", control.Nchan)
	fmt.Println("Channel groups per ring:  ", control.Ngroups)
	fmt.Println("Channel # of 1st chan:    ", control.Chan0)
	if control.Ngroups > 1 || control.Nrings > 1 {
		fmt.Println("Skip # btwn groups/rings: ", control.chanGaps)
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
			amplitude := 1000.0
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
			amplitude := 5000.0 + 200.0*float64(cnum)
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
		FractionBits := 13
		if FractionBits < 16 {
			for j := 0; j < Nsamp; j++ {
				raw := d[i+Nchan*j]
				d[i+Nchan*j] = raw % (1 << FractionBits)
			}
		}
	}

	dims := []int16{int16(Nchan)}
	timer := time.NewTicker(repeatTime)
	timeCounter := uint64(0)
	for {
		select {
		case <-cancel:
			close(packetchan)
			return nil
		case <-timer.C:
			for i := 0; i < Nchan*Nsamp; i += valuesPerPacket {
				// Must be careful: the last iteration might have fewer samples than the others.
				lastsamp := i + valuesPerPacket
				if lastsamp > Nchan*Nsamp {
					lastsamp = Nchan * Nsamp
				}
				packet.NewData(d[i:lastsamp], dims)
				ts := packets.MakeTimestamp(uint16(timeCounter>>32), uint32(timeCounter), counterRate)
				packet.SetTimestamp(ts)
				if control.dropfrac == 0.0 || control.dropfrac < randsource.Float64() {
					packetchan <- packet.Bytes()
				}
				timeCounter += countsPerSample * uint64((lastsamp-i)/Nchan)
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
	nchan := flag.Int("nchan", 4, "Number of channels per ring, 4-512 allowed")
	nring := flag.Int("nring", 1, "Number of ring buffers, 1-4 allowed")
	chan0 := flag.Int("firstchan", 0, "Channel number of the first channel")
	changaps := flag.Int("gaps", 0, "How many channel numbers to skip between groups (relevant only if ngroups>1 or nring>1)")
	ngroups := flag.Int("ngroups", 1, "Number of channel groups per ring, 1-Nchan/4 allowed")
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
		fmt.Println("If none of noise, saw, or pulse are given, saw will be used.")
	}
	flag.Parse()

	clearRings(maxRings)
	coerceInt(nchan, 4, 512)
	coerceInt(nring, 1, 4)
	coerceInt(ngroups, 1, *nchan/4)
	coerceInt(changaps, 0, math.MaxInt64)
	coerceFloat(samplerate, 1000, 500000)
	coerceFloat(droppct, 0, 100)

	// Compute the size of each ring buffer. Let it be big enough to hold 0.5 seconds of data
	// or 500 packets, and be a multiple of packetAlign.
	ringlasts := 0.5  // seconds
	byterate := 2.0*float64(*nchan)*(*samplerate)
	ringsize := int(byterate*ringlasts) + packetAlign
	ringsize -= ringsize % packetAlign
	if ringsize < 500*packetAlign {
		ringsize = 500*packetAlign
	}

	control := BahamaControl{Nchan: *nchan, Ngroups: *ngroups, Nrings: *nring,
		Chan0: *chan0, chanGaps: *changaps, ringsize: ringsize,
		stagger: *stagger, interleave: *interleave,
		sawtooth: *usesawtooth, pulses: *usepulses,
		sinusoid: *usesine, noiselevel: *noiselevel,
		samplerate: *samplerate, dropfrac: (*droppct) / 100.0}

	control.Report()

	cancel := make(chan os.Signal)
	signal.Notify(cancel, os.Interrupt, syscall.SIGTERM)
	ch0 := control.Chan0
	for cardnum := 0; cardnum < *nring; cardnum++ {
		packetchan := make(chan []byte)
		if err := ringwriter(cardnum, packetchan, ringsize); err != nil {
			fmt.Printf("ringwriter(%d,...) failed: %v\n", cardnum, err)
			continue
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
