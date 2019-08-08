package dastard

import (
	"fmt"
	"io"
	"os"
)

// Pixel represents the physical location of a TES
type Pixel struct {
	X, Y int
	Name string
}

// Map represents an entire array of pixel locations
type Map struct {
	Spacing  int
	Pixels   []Pixel
	Filename string
}

func readMap(filename string) (*Map, error) {
	m := new(Map)
	m.Pixels = make([]Pixel, 0)
	m.Filename = filename

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := fmt.Fscanf(file, "spacing: %d\n", &m.Spacing); err != nil {
		return nil, err
	}

	for {
		var chnum int
		var p Pixel
		_, err := fmt.Fscanf(file, "%d %d %d %s", &chnum, &p.X, &p.Y, &p.Name)
		if err == io.EOF {
			return m, nil
		}
		if err != nil {
			fmt.Println(m)
			fmt.Println(chnum, p)
			return m, err
		}
		m.Pixels = append(m.Pixels, p)
	}
}

// MapServer is the RPC service that loads and broadcasts TES maps
type MapServer struct {
	m             *Map
	clientUpdates chan<- ClientUpdate
}

func newMapServer() *MapServer {
	return new(MapServer)
}

// Load reads a map file and broadcasts it to clients
func (ms *MapServer) Load(filename *string, reply *bool) error {
	m, err := readMap(*filename)
	*reply = err == nil
	if err != nil {
		return err
	}
	ms.m = m
	ms.broadcastMap()
	return nil
}

func (ms *MapServer) broadcastMap() {
	ms.clientUpdates <- ClientUpdate{"TESMAPFILE", ms.m.Filename}
	ms.clientUpdates <- ClientUpdate{"TESMAP", ms.m}
}
