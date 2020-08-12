package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fabiokung/shm"
)

func TestHelpers(t *testing.T) {
	clearRings(4)
	f := 1.0
	j := 1
	mins := []float64{0, -10, 10}
	maxs := []float64{2, 0, 20}
	expect := []float64{1, 0, 10}
	for i := range(mins) {
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
	ngroup := 3
	c1 := make([]chan []byte, ngroup)
	p := make([][]byte, ngroup)
	for i := 0; i<ngroup; i++ {
		c1[i] = make(chan []byte)
		p[i] = []byte{byte(i)}
	}
	c2 := make(chan []byte)
	go interleavePackets(c2, c1, false)

	npackets := 24
	for j := 0; j<ngroup; j++ {
		go func(cid int) {
			for i := 0; i<npackets; i++ {
				c1[cid] <- p[cid]
			}
			close(c1[cid])
		}(j)
	}
	for i := 0; i<npackets*ngroup; i++ {
		pi, ok := <-c2
		if !ok {
			t.Errorf("Expected %d non-staggered packets before output channel closed, got %d", npackets*ngroup, i)
		}
		expect := byte((i/4) % ngroup)
		if pi[0] != expect {
			t.Errorf("Non-staggered interleave packet %3d source is %d, want %d", i, pi[0], expect)
		}
	}
	if _, ok := <-c2; ok {
		t.Errorf("Expected interleavePackets to close output channel.")
	}
}

func TestGenerate(t *testing.T) {
	const cardnum = -3
	cancel := make(chan os.Signal)
	go func() {
		time.Sleep(40 * time.Millisecond)
		close(cancel)
	}()
	// control := BahamaControl{Nchan:4, Ngroups:1, sinusoid:true, sawtooth:false, noiselevel:5.0, samplerate:100000}
	// err := generateData(cardnum, cancel, control)
	// if err != nil {
	// 	t.Errorf("generateData() returned %s", err.Error())
	// }

	// Ensure that the above deleted the shared memory region
	name := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	if region, err := shm.Open(name, os.O_RDONLY, 0600); err == nil {
		region.Close()
		shm.Unlink(name)
		t.Errorf("generateData() left shm:%s in existence", name)
	}
}
