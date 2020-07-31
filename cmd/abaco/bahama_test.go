package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fabiokung/shm"
)

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
