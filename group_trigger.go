package dastard

import (
	"fmt"
	"sort"
	"sync"
)

// type receiverSet map[int]bool

type triggerList struct {
	channum int
	frames  []int64
}

// TriggerBroker communicates with DataChannel objects to allow them to operate independently
// yet still share group triggering information.
type TriggerBroker struct {
	nchannels       int
	sources         []map[int]bool
	PrimaryTrigs    chan triggerList
	SecondaryTrigs  []chan []int64
	latestPrimaries [][]int64
	lock            sync.RWMutex
}

// NewTriggerBroker creates a new TriggerBroker object for nchan channels to share group triggers.
func NewTriggerBroker(nchan int) *TriggerBroker {
	broker := new(TriggerBroker)
	broker.nchannels = nchan
	broker.sources = make([]map[int]bool, nchan)
	for i := 0; i < nchan; i++ {
		broker.sources[i] = make(map[int]bool)
	}
	broker.PrimaryTrigs = make(chan triggerList, nchan)
	broker.SecondaryTrigs = make([]chan []int64, nchan)
	for i := 0; i < nchan; i++ {
		broker.SecondaryTrigs[i] = make(chan []int64, 1)
	}
	broker.latestPrimaries = make([][]int64, nchan)
	return broker
}

// AddConnection connects source -> receiver for group triggers
func (broker *TriggerBroker) AddConnection(source, receiver int) error {
	if receiver < 0 || receiver >= broker.nchannels {
		return fmt.Errorf("Could not add channel %d as a group receiver (nchannels=%d)",
			receiver, broker.nchannels)
	}
	broker.lock.Lock()
	broker.sources[receiver][source] = true
	broker.lock.Unlock()
	return nil
}

// DeleteConnection disconnects source -> receiver for group triggers
func (broker *TriggerBroker) DeleteConnection(source, receiver int) error {
	if receiver < 0 || receiver >= broker.nchannels {
		return fmt.Errorf("Could not remove channel %d as a group receiver (nchannels=%d)",
			receiver, broker.nchannels)
	}
	broker.lock.Lock()
	delete(broker.sources[receiver], source)
	broker.lock.Unlock()
	return nil
}

// isConnected returns whether source->receiver is connected.
func (broker *TriggerBroker) isConnected(source, receiver int) bool {
	if receiver < 0 || receiver >= broker.nchannels {
		return false
	}
	broker.lock.RLock()
	_, ok := broker.sources[receiver][source]
	broker.lock.RUnlock()
	return ok
}

// Connections returns a set of all sources for the given receiver.
func (broker *TriggerBroker) Connections(receiver int) map[int]bool {
	if receiver < 0 || receiver >= broker.nchannels {
		return nil
	}
	broker.lock.RLock()
	sources := broker.sources[receiver]
	broker.lock.RUnlock()
	return sources
}

// Int64Slice attaches the methods of sort.Interface to []int64, sorting in increasing order.
type Int64Slice []int64

func (p Int64Slice) Len() int           { return len(p) }
func (p Int64Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p Int64Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Run runs in a goroutine to broker trigger frame #s from sources to receivers.
// It runs in the pattern: get a message from each channel (about their triggered
// frame numbers), then send a message to each channel (about their secondary triggers).
func (broker *TriggerBroker) Run(abort <-chan struct{}) {
	for {
		// get data from all PrimaryTrigs channels
		for i := 0; i < broker.nchannels; i++ {
			select {
			case <-abort:
				// fmt.Println("I was told to abort")
				return
			case tlist := <-broker.PrimaryTrigs:
				broker.latestPrimaries[tlist.channum] = tlist.frames
			}
		}

		// send reponse to all SecondaryTrigs channels
		broker.lock.RLock()
		for idx, rxchan := range broker.SecondaryTrigs {
			sources := broker.Connections(idx)
			var trigs []int64
			if len(sources) > 0 {
				for source := range sources {
					trigs = append(trigs, broker.latestPrimaries[source]...)
				}
				sort.Sort(Int64Slice(trigs))
			}
			rxchan <- trigs
		}
		broker.lock.RUnlock()
	}
}
