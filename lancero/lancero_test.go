package lancero

import (
	"fmt"
	"testing"
)

func TestProbe(t *testing.T) {
	devs, err := EnumerateLanceroDevices()
	fmt.Printf("Lancero devices: %v\n", devs)
	if err != nil {
		t.Errorf("EnumerateLanceroDevices() failed with err=%s", err.Error())
	}
}
