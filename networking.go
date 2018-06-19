package dastard

// Portnumbers holds all TCP port numbers used by Dastard.
type Portnumbers struct {
	RPC            int
	Status         int
	Trigs          int
	SecondaryTrigs int
	Summaries      int
}

var ports = Portnumbers{5500, 5501, 5502, 5503, 5504}

func setPortnumbers(base int) {
	ports.RPC = base
	ports.Status = base + 1
	ports.Trigs = base + 2
	ports.SecondaryTrigs = base + 3
	ports.Summaries = base + 4
}
