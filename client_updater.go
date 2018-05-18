package dastard

// Contain the ClientUpdater object, which publishes JSON-encoded messages
// giving the latest DASTARD state.

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/spf13/viper"
	czmq "github.com/zeromq/goczmq"
)

// ClientUpdate carries the messages to be published on the status port.
type ClientUpdate struct {
	tag   string
	state interface{}
}

// RunClientUpdater forwards any message from its input channel to the ZMQ publisher socket
// to publish any information that clients need to know.
func RunClientUpdater(messages <-chan ClientUpdate, portstatus int) {
	hostname := fmt.Sprintf("tcp://*:%d", portstatus)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return
	}
	defer pubSocket.Destroy()

	// Save the state to the standard saved-state file this often.
	// TODO: change to time.Minute
	savePeriod := time.Second * 3
	saveStateTicker := time.NewTicker(savePeriod)
	defer saveStateTicker.Stop()

	// Here, store the last message of each type seen. Use when storing state.
	lastMessages := make(map[string]interface{})

	for {
		select {
		case update := <-messages:
			lastMessages[update.tag] = update.state
			message, err := json.Marshal(update.state)
			if err != nil {
				continue
			}
			topic := reflect.TypeOf(update.state).String()
			log.Printf("Here is message of type %s: %v\n", topic, string(message))

			pubSocket.SendFrame([]byte(update.tag), czmq.FlagMore)
			pubSocket.SendFrame(message, czmq.FlagNone)
		case <-saveStateTicker.C:
			saveState(lastMessages)
		}
	}
}

// saveState stores server configuration to the standard config file.
func saveState(lastMessages map[string]interface{}) {
	lastMessages["CURRENTTIME"] = time.Now().Format(time.UnixDate)
	for k, v := range lastMessages {
		viper.Set(k, v)
	}
	// TODO: save to a new file and then move into place. Perhaps WriteConfigAs(tmpfile)?
	viper.WriteConfig()
}
