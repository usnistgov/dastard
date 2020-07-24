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
	const Nchan = 4
	const sine = true
	const saw = false
	const noise = 5.0
	err := generateData(cardnum, cancel, Nchan, sine, saw, noise)
	if err != nil {
		t.Errorf("generateData() returned %s", err.Error())
	}

	// Ensure that the above deleted the shared memory region
	name := fmt.Sprintf("xdma%d_c2h_0_description", cardnum)
	if region, err := shm.Open(name, os.O_RDONLY, 0600); err == nil {
		region.Close()
		shm.Unlink(name)
		t.Errorf("generateData() left shm:%s in existence", name)
	}
}
