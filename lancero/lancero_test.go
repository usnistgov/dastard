package lancero

import (
	"testing"
)

func TestProbe(t *testing.T) {
	devs, err := EnumerateLanceroDevices()
	t.Logf("Lancero devices: %v\n", devs)
	if err != nil {
		t.Errorf("EnumerateLanceroDevices() failed with err=%s", err.Error())
	}
}
