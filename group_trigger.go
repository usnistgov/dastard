package dastard

import "fmt"

type receiverSet map[int]bool

// TriggerBroker communicates with DataChannel objects to allow them to operate independently
// yet still share group triggering information.
type TriggerBroker struct {
	nchannels int
	receivers []map[int]bool
}

// NewTriggerBroker creates a new TriggerBroker object for nchan channels to share group triggers.
func NewTriggerBroker(nchan int) *TriggerBroker {
	broker := new(TriggerBroker)
	broker.nchannels = nchan
	broker.receivers = make([]map[int]bool, nchan)
	for i := 0; i < nchan; i++ {
		broker.receivers[i] = make(map[int]bool)
	}
	return broker
}

// AddConnection connects source -> receiver for group triggers
func (broker *TriggerBroker) AddConnection(source, receiver int) error {
	if source < 0 || source >= broker.nchannels {
		return fmt.Errorf("Could not add channel %d as a group source (nchannels=%d)",
			source, broker.nchannels)
	}
	broker.receivers[source][receiver] = true
	return nil
}

// DeleteConnection disconnects source -> receiver for group triggers
func (broker *TriggerBroker) DeleteConnection(source, receiver int) error {
	if source < 0 || source >= broker.nchannels {
		return fmt.Errorf("Could not remove channel %d as a group source (nchannels=%d)",
			source, broker.nchannels)
	}
	delete(broker.receivers[source], receiver)
	return nil
}

// isConnected returns whether source->receiver is connected.
func (broker *TriggerBroker) isConnected(source, receiver int) bool {
	if source < 0 || source >= broker.nchannels {
		return false
	}
	_, ok := broker.receivers[source][receiver]
	return ok
}

// Connections returns a set of all receivers for the given source.
func (broker *TriggerBroker) Connections(source int) map[int]bool {
	if source < 0 || source >= broker.nchannels {
		return nil
	}
	return broker.receivers[source]
}
