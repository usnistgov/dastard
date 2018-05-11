package dastard

// Contain the ClientUpdater object, which publishes JSON-encoded messages
// giving the latest DASTARD state.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/user"
	"reflect"
	"time"

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
	lastMessages := make(map[string]string)

	// Where do we store configuration dfiles?
	u, err := user.Current()
	configDirname := u.HomeDir + "/.dastard/"
	err = os.Mkdir(configDirname, os.FileMode(0775))
	if err != nil && !os.IsExist(err) {
		log.Println("Could not make ~/.dastard directory: ", err)
		return
	}

	for {
		select {
		case update := <-messages:
			message, err := json.Marshal(update.state)
			if err != nil {
				continue
			}
			topic := reflect.TypeOf(update.state).String()
			fmt.Printf("Here is message of type %s: %v\n", topic, string(message))

			lastMessages[update.tag] = string(message)
			pubSocket.SendFrame([]byte(update.tag), czmq.FlagMore)
			pubSocket.SendFrame(message, czmq.FlagNone)
		case <-saveStateTicker.C:
			saveState(configDirname, lastMessages)
		}
	}
}

func saveState(configDirname string, lastMessages map[string]string) {
	fname := configDirname + "dastard.cfg"
	tmpname := configDirname + "dastard.tmp"
	bakname := configDirname + "dastard.cfg.bak"

	fp, err := os.Create(tmpname)
	if err != nil {
		log.Println("Could not write dastard.cfg file: ", err)
		return
	}
	state, err := json.MarshalIndent(lastMessages, "", "    ")
	if err != nil {
		log.Println("Could not write convert dastard.cfg information to JSON: ", err)
	} else {
		fmt.Fprint(fp, string(state))
	}
	fp.Close()

	// Move old config file to backup and new file to standard config name.
	err = os.Remove(bakname)
	if err != nil && !os.IsNotExist(err) {
		log.Println("Could not remove backup file ", bakname, " even though it exists.")
		log.Println(err)
		return
	}
	err = os.Rename(fname, bakname)
	if err != nil && !os.IsNotExist(err) {
		log.Println("Could not save backup file: ", err)
		return
	}
	err = os.Rename(tmpname, fname)
	if err != nil {
		log.Println("Could not update dastard.cfg file")
	}
}
