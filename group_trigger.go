package dastard

import (
	"fmt"
	"sync"
)

type receiverSet map[int]bool

// TriggerBroker communicates with DataChannel objects to allow them to operate independently
// yet still share group triggering information.
type TriggerBroker struct {
	nchannels       int
	sources         []map[int]bool
	PrimaryTrigs    chan []int64
	SecondaryTrigs  chan []int64
	latestPrimaries [][]int
	lock            sync.Mutex
}

// NewTriggerBroker creates a new TriggerBroker object for nchan channels to share group triggers.
func NewTriggerBroker(nchan int) *TriggerBroker {
	broker := new(TriggerBroker)
	broker.nchannels = nchan
	broker.sources = make([]map[int]bool, nchan)
	for i := 0; i < nchan; i++ {
		broker.sources[i] = make(map[int]bool)
	}
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
	broker.lock.Lock()
	_, ok := broker.sources[receiver][source]
	broker.lock.Unlock()
	return ok
}

// Connections returns a set of all sources for the given receiver.
func (broker *TriggerBroker) Connections(receiver int) map[int]bool {
	if receiver < 0 || receiver >= broker.nchannels {
		return nil
	}
	broker.lock.Lock()
	sources := broker.sources[receiver]
	broker.lock.Unlock()
	return sources
}

// Run runs in a goroutine to broker trigger frame #s from sources to receivers
func (broker *TriggerBroker) Run(abort <-chan struct{}) {
	for {
		// get data from all PrimaryTrigs channels

		// send reponse to all SecondaryTrigs channels

		// check for close... how?
	}
}
