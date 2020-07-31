package main

import (
	"fmt"
	"flag"
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

func clearRings(nclear int) error {
	for cardnum := 0; cardnum < nclear; cardnum++ {
		ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
		ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
		ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
		if err != nil {
			return fmt.Errorf("Could not open ringbuffer: %s", err)
		}
		ring.Unlink()       // in case it exists from before
	}
	return nil
}

// BahamaControl carries all the free parameters of the data generator
type BahamaControl struct {
	Nchan int
	sinusoid bool
	sawtooth bool
	pulses bool
	noiselevel float64
	samplerate float64
}


func generateData(cardnum int, cancel chan os.Signal, control BahamaControl) error {
	ringname := fmt.Sprintf("xdma%d_c2h_0_buffer", cardnum)
	ringdesc := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	ring, err := ringbuffer.NewRingBuffer(ringname, ringdesc)
	if err != nil {
		return fmt.Errorf("Could not open ringbuffer: %s", err)
	}
	ring.Unlink()       // in case it exists from before
	defer ring.Unlink() // so it won't exist after
	const packetAlign = 8192
	if err = ring.Create(256 * packetAlign); err != nil {
		return fmt.Errorf("Failed RingBuffer.Create: %s", err)
	}

	// Data will play on infinite repeat with this many samples and this repeat period:
	const Nsamp = 40000  // This many samples before data repeats itself
	sampleRate := control.samplerate
	periodns := float64(Nsamp)/sampleRate*1e9
	repeatTime := time.Duration(periodns+0.5)*time.Nanosecond // Repeat data with this period

	Nchan := control.Nchan
	stride := 4000 / Nchan // We'll put this many samples into each packet
	valuesPerPacket := stride * Nchan
	if 2*valuesPerPacket > 8000 {
		return fmt.Errorf("Packet payload size %d exceeds 8000 bytes", 2*valuesPerPacket)
	}
	const counterRate = 1e8 // counts per second
	countsPerSample := uint64(counterRate/sampleRate+0.5)

 	randsource := rand.New(rand.NewSource(time.Now().UnixNano()))

	fmt.Printf("Generating data in shm:%s\n", ringname)
	p := packets.NewPacket(10, 20, 0x100, 0)
	d := make([]int16, Nchan*Nsamp)
	for i := 0; i < control.Nchan; i++ {
		offset := int16(i*1000)
		for j := 0; j < Nsamp; j++ {
			d[i+Nchan*j] = offset
		}
		if control.sinusoid {
			freq := (float64(i+1) * 2 * math.Pi) / float64(Nsamp)
			amplitude := 1000.0
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(amplitude*math.Sin(freq*float64(j)))
			}
		}
		if control.sawtooth {
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(j%5000)
			}
		}
		if control.pulses {
			amplitude := 5000.0+200.0*float64(Nchan)
			scale := amplitude*2.116
			for j := 0; j < Nsamp / 4; j++ {
				pulsevalue := int16(scale*(math.Exp(-float64(j)/1200.0)-math.Exp(-float64(j)/300.0)))
				// Same pulse repeats 4x
				for k := 0; k < 4; k++ {
					d[i+Nchan*(j+k*(Nsamp/4))] += pulsevalue
				}
			}
		}
		if control.noiselevel > 0.0 {
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(randsource.NormFloat64()*control.noiselevel)
			}
		}

		// Abaco data only records the lowest N bits.
		// Wrap the 16-bit data properly into [0, (2^N)-1]
		FractionBits := 13
		if FractionBits < 16 {
			for j := 0; j < Nsamp; j++ {
				raw := d[i+Nchan*j]
				d[i+Nchan*j] = raw % (1<<FractionBits)
			}
		}

	}

	empty := make([]byte, packetAlign)
	dims := []int16{int16(Nchan)}
	timer := time.NewTicker(repeatTime)
	timeCounter := uint64(0)
	for {
		select {
		case <-cancel:
			return nil
		case <-timer.C:
			for i := 0; i < Nchan*Nsamp; i += valuesPerPacket {
				// Must be careful: the last iteration might have fewer samples than the others.
				lastsamp := i+valuesPerPacket
				if lastsamp > Nchan*Nsamp {
					lastsamp = Nchan*Nsamp
				}
				p.NewData(d[i:lastsamp], dims)
				ts := packets.MakeTimestamp(uint16(timeCounter>>32), uint32(timeCounter), counterRate)
				p.SetTimestamp(ts)
				b := p.Bytes()
				b = append(b, empty[:packetAlign-len(b)]...)
				if ring.BytesWriteable() >= len(b) {
					ring.Write(b)
				}
				timeCounter += countsPerSample*uint64((lastsamp-i)/Nchan)
			}
		}
	}
}

func main() {
	maxRings := 4
	nchan := flag.Int("nchan", 4, "Number of channels per ring, 4-512 allowed")
	nring := flag.Int("nring", 1, "Number of ring buffers, 1-4 allowed")
	samplerate := flag.Float64("rate", 200000., "Samples per channel per second, 100-500000")
	noiselevel := flag.Float64("noise", 0.0, "White noise level (<=0 means no noise)")
	usesawtooth := flag.Bool("saw", false, "Whether to add a sawtooth pattern")
	usesine := flag.Bool("sine", false, "Whether to add a sinusoidal pattern")
	usepulses := flag.Bool("pulse", false, "Whether to add pulse-like data")
	flag.Usage = func() {
		fmt.Println("BAHAMA, the Basic Abaco Hardware Artificial Message Assembler")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		fmt.Println("If none of noise, saw, or pulse are given, saw will be used.")
	}
 	flag.Parse()
	clearRings(maxRings)
	if *nring >= maxRings {
		*nring = maxRings-1
	}
	if *samplerate > 500000 {
		*samplerate = 500000
	}
	if *samplerate < 100 {
		*samplerate = 100
	}

	control := BahamaControl{Nchan:*nchan, sawtooth:*usesawtooth, pulses:*usepulses,
		sinusoid:*usesine, noiselevel:*noiselevel, samplerate:*samplerate}

	fmt.Println("Number of ring buffers: ", *nring)
	fmt.Println("Channels per ring:      ", control.Nchan)
	fmt.Println("Samples per second:     ", *samplerate)
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

	cancel := make(chan os.Signal)
	signal.Notify(cancel, os.Interrupt, syscall.SIGTERM)
	for cardnum := 0; cardnum < *nring; cardnum++ {
		go func(cn int) {
			if err := generateData(cn, cancel, control); err != nil {
				fmt.Printf("generateData(%d,...) failed: %v\n", cn, err)
			}
		}(cardnum)
	}
	<-cancel
}
