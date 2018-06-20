package dastard

// Portnumbers structs can contain all TCP port numbers used by Dastard.
type Portnumbers struct {
	RPC            int
	Status         int
	Trigs          int
	SecondaryTrigs int
	Summaries      int
}

// Ports globally holds all TCP port numbers used by Dastard.
var Ports Portnumbers

func init() {
	setPortnumbers(5500)
}

func setPortnumbers(base int) {
	Ports.RPC = base
	Ports.Status = base + 1
	Ports.Trigs = base + 2
	Ports.SecondaryTrigs = base + 3
	Ports.Summaries = base + 4
}
