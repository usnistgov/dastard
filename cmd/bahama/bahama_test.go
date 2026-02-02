package main

import (
	"os"
	"testing"
	"time"
)

func TestHelpers(t *testing.T) {
	f := 1.0
	j := 1
	mins := []float64{0, -10, 10}
	maxs := []float64{2, 0, 20}
	expect := []float64{1, 0, 10}
	for i := range mins {
		coerceFloat(&f, mins[i], maxs[i])
		if f != expect[i] {
			t.Errorf("coerceInt made f=%.4f, want %.4f", f, expect[i])
		}
		coerceInt(&j, int(mins[i]), int(maxs[i]))
		e := int(expect[i])
		if j != e {
			t.Errorf("coerceInt made f=%d, want %d", j, e)
		}
	}
}

func TestInterleave(t *testing.T) {
	const ngroup = 3
	const npackets = 28

	// First test interleaved, non-staggered packets
	c1 := make([]chan []byte, ngroup)
	p := make([][]byte, ngroup)
	for i := range ngroup {
		c1[i] = make(chan []byte)
		p[i] = []byte{byte(i)}
	}
	c2 := make(chan []byte)
	go interleavePackets(c2, c1, false)

	for j := range ngroup {
		go func(cid int) {
			for range npackets {
				c1[cid] <- p[cid]
			}
			close(c1[cid])
		}(j)
	}
	for i := range npackets * ngroup {
		pi, ok := <-c2
		if !ok {
			t.Errorf("Expected %d non-staggered packets before output channel closed, got %d", npackets*ngroup, i)
		}
		expect := byte((i / 4) % ngroup)
		if pi[0] != expect {
			t.Errorf("Non-staggered interleave packet %3d source is %d, want %d", i, pi[0], expect)
		}
	}
	if _, ok := <-c2; ok {
		t.Errorf("Expected interleavePackets to close output channel.")
	}

	// Now test staggered, interleaved packets
	c1 = make([]chan []byte, ngroup)
	for i := range ngroup {
		c1[i] = make(chan []byte)
	}
	c2 = make(chan []byte)
	go interleavePackets(c2, c1, true)
	for j := range ngroup {
		go func(cid int) {
			for range npackets {
				c1[cid] <- p[cid]
			}
			close(c1[cid])
		}(j)
	}
	expectby4 := []byte{0, 1, 1, 2, 2, 2, 0, 0, 0, 1, 1, 1, 2, 2, 2, 0, 0, 0, 1, 1, 2}
	for i := range npackets * ngroup {
		pi, ok := <-c2
		if !ok {
			t.Errorf("Expected %d staggered packets before output channel closed, got %d", npackets*ngroup, i)
		}
		expect := expectby4[i/4]
		if pi[0] != expect {
			t.Errorf("Staggered interleave packet %3d source is %d, want %d", i, pi[0], expect)
		}
	}
	if _, ok := <-c2; ok {
		t.Errorf("Expected interleavePackets to close output channel.")
	}
}

func TestGenerate(t *testing.T) {
	cancel := make(chan os.Signal)
	go func() {
		time.Sleep(40 * time.Millisecond)
		close(cancel)
	}()
	control := BahamaControl{Nchan: 4, Ngroups: 1, sinusoid: true, sawtooth: true, pulses: true,
		noiselevel: 5.0, samplerate: 100000}

	// Keep the data channel drained...
	ch := make(chan []byte)
	go func() {
		for {
			<-ch
		}
	}()
	err := generateData(control.Nchan, 0, ch, cancel, control)
	if err != nil {
		t.Errorf("generateData() returned %s", err.Error())
	}

}

func BenchmarkUDPGenerate(b *testing.B) {
	cancel := make(chan os.Signal)
	go func() {
		time.Sleep(60 * time.Second)
		close(cancel)
	}()

	packetchan := make(chan []byte)

	control := BahamaControl{Nchan: 64, Ngroups: 1, Nsources: 1, pulses: true,
		noiselevel: 5.0, samplerate: 244140, port: 4000}
	if err := udpwriter(control.port, packetchan); err != nil {
		b.Errorf("udpwriter(%d,...) failed: %v\n", control.port, err)
	}
	if err := generateData(control.Nchan, 0, packetchan, cancel, control); err != nil {
		b.Errorf("generateData() returned %s", err.Error())
	}
}
