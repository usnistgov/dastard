package dastard

// Contain the ClientUpdater object, which publishes JSON-encoded messages
// giving the latest DASTARD state.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/viper"
	czmq "github.com/zeromq/goczmq"
)

// ClientUpdate carries the messages to be published on the status port.
type ClientUpdate struct {
	tag   string
	state interface{}
}

func publish(pubSocket *czmq.Sock, update ClientUpdate) {
	message, err := json.Marshal(update.state)
	if err != nil {
		return
	}
	topic := reflect.TypeOf(update.state).String()
	log.Printf("Message of type %s: %v\n", topic, string(message))

	pubSocket.SendFrame([]byte(update.tag), czmq.FlagMore)
	pubSocket.SendFrame(message, czmq.FlagNone)
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
			if update.tag == "SENDALL" {
				for k, v := range lastMessages {
					publish(pubSocket, ClientUpdate{tag: k, state: v})
				}
				continue
			}
			lastMessages[update.tag] = update.state
			publish(pubSocket, update)
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

	mainname := viper.ConfigFileUsed()
	tmpname := strings.Replace(mainname, ".yaml", ".tmp.yaml", 1)
	bakname := mainname + ".bak"
	err := viper.WriteConfigAs(tmpname)
	if err != nil {
		log.Println("Could not store config file ", tmpname, ": ", err)
		return
	}

	// Move old config file to backup and new file to standard config name.
	err = os.Remove(bakname)
	if err != nil && !os.IsNotExist(err) {
		log.Println("Could not remove backup file ", bakname, " even though it exists: ", err)
		return
	}
	err = os.Rename(mainname, bakname)
	if err != nil && !os.IsNotExist(err) {
		log.Println("Could not save backup file: ", err)
		return
	}
	err = os.Rename(tmpname, mainname)
	if err != nil {
		log.Printf("Could not update dastard config file %s", mainname)
	}

}
