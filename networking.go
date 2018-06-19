package dastard

// Portnumbers holds all TCP port numbers used by Dastard.
type Portnumbers struct {
	RPC            int
	Status         int
	Trigs          int
	SecondaryTrigs int
	Summaries      int
}

var Ports = Portnumbers{5500, 5501, 5502, 5503, 5504}

func setPortnumbers(base int) {
	Ports.RPC = base
	Ports.Status = base + 1
	Ports.Trigs = base + 2
	Ports.SecondaryTrigs = base + 3
	Ports.Summaries = base + 4
}
