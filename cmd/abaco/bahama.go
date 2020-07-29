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


func generateData(cardnum int, cancel chan os.Signal, Nchan int, sinusoid bool, sawtooth bool, noiselevel float64) error {
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

	const Nsamp = 10000  // This many samples before data repeats itself
	const repeatTime = 40*time.Millisecond // Repeat data with this period
	stride := 4000 / Nchan // We'll put this many samples into a packet
	valuesPerPacket := stride * Nchan
	if 2*valuesPerPacket > 8000 {
		return fmt.Errorf("Packet payload size %d exceeds 8000 bytes", 2*valuesPerPacket)
	}
	sampleRate := float64(Nsamp)/(float64(repeatTime)/1e9)
	const counterRate = 1e8 // counts per second
	countsPerSample := uint64(counterRate/sampleRate+0.5)

 	randsource := rand.New(rand.NewSource(time.Now().UnixNano()))

	fmt.Printf("Generating data in shm:%s\n", ringname)
	p := packets.NewPacket(10, 20, 0x100, 0)
	d := make([]int16, Nchan*Nsamp)
	for i := 0; i < Nchan; i++ {
		offset := int16(i*1000)
		for j := 0; j < Nsamp; j++ {
			d[i+Nchan*j] = offset
		}
		if sinusoid {
			freq := (float64(i+1) * 2 * math.Pi) / float64(Nsamp)
			amplitude := 1000.0
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(amplitude*math.Sin(freq*float64(j)))
			}
		}
		if sawtooth {
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(j%5000)
			}
		}
		if noiselevel > 0.0 {
			for j := 0; j < Nsamp; j++ {
				d[i+Nchan*j] += int16(randsource.NormFloat64()*noiselevel)
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
	// samplerate := flag.Float64("rate", 10000., "Samples per channel per second, 100-400000")
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

	fmt.Println("Number of ring buffers: ", *nring)
	fmt.Println("Channels per ring:      ", *nchan)
	// fmt.Println("Samples per second:     ", *samplerate)
	if !(*noiselevel > 0.0 || *usesawtooth || *usepulses || *usesine) {
		*usesawtooth = true
	}
	var sources []string
	if *noiselevel > 0.0 {
		sources = append(sources, "noise")
	}
	if *usesawtooth {
		sources = append(sources, "sawtooth")
	}
	if *usepulses {
		sources = append(sources, "pulses")
	}
	if *usesine {
		sources = append(sources, "sinusoids")
	}
	fmt.Printf("Data will be the sum of these source types: %s.\n", strings.Join(sources, "+"))
	fmt.Println("Type Ctrl-C to stop generating Abaco-style data.")

	cancel := make(chan os.Signal)
	signal.Notify(cancel, os.Interrupt, syscall.SIGTERM)
	for cardnum := 0; cardnum < *nring; cardnum++ {
		go func(cn int) {
			if err := generateData(cn, cancel, *nchan, *usesine, *usesawtooth, *noiselevel); err != nil {
				fmt.Printf("generateData(%d,...) failed: %v\n", cn, err)
			}

		}(cardnum)
	}
	<-cancel
}
