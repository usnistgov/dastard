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

func publish(pubSocket *czmq.Sock, update ClientUpdate, message []byte) {
	topic := reflect.TypeOf(update.state).String()
	if topic != "TRIGGERRATE" {
		log.Printf("SEND Message of type %s: %v\n", topic, string(message))
	}
	pubSocket.SendFrame([]byte(update.tag), czmq.FlagMore)
	pubSocket.SendFrame(message, czmq.FlagNone)
}

var clientMessageChan chan ClientUpdate

func init() {
	clientMessageChan = make(chan ClientUpdate, 10)
}

// RunClientUpdater forwards any message from its input channel to the ZMQ publisher socket
// to publish any information that clients need to know.
func RunClientUpdater(statusport int) {
	hostname := fmt.Sprintf("tcp://*:%d", statusport)
	pubSocket, err := czmq.NewPub(hostname)
	if err != nil {
		return
	}
	defer pubSocket.Destroy()

	// The ZMQ middleware will need some time for existing SUBscribers (and their
	// subscription topics) to be hooked up to this new PUBlisher.
	// The result is that the first few messages will be dropped, including the
	// NEWDASTARD one. By sleeping a fraction of a second, we can avoid this
	// dropped-message problem most of the time (though there's no guarantee).
	time.Sleep(250 * time.Millisecond)

	// Save the state to the standard saved-state file this often.
	savePeriod := time.Minute
	saveStateRegularlyTicker := time.NewTicker(savePeriod)
	defer saveStateRegularlyTicker.Stop()

	// And also save state every time it's changed, but after a delay of this long.
	saveDelayAfterChange := time.Second * 2
	saveStateOnceTimer := time.NewTimer(saveDelayAfterChange)

	// Here, store the last message of each type seen. Use when storing state.
	lastMessages := make(map[string]interface{})
	lastMessageStrings := make(map[string]string)

	for {
		select {
		case update := <-clientMessageChan:
			if update.tag == "SENDALL" {
				for k, v := range lastMessages {
					publish(pubSocket, ClientUpdate{tag: k, state: v}, []byte(lastMessageStrings[k]))
				}
				continue
			}

			// Send state to clients now.
			message, err := json.Marshal(update.state)
			if err == nil {
				publish(pubSocket, update, message)
			}

			// Don't save NEWDASTARD messages--they don't contain state
			if update.tag == "NEWDASTARD" {
				continue
			}

			// Check if the state has changed; if so, remember the message for later
			// (we'll need to broadcast it when a new client asks for a SENDALL).
			// If it's also NOT on the no-save list, save to Viper config file after a delay.
			// The delay allows us to accumulate many near-simultaneous changes then
			// save only once.
			updateString := string(message)
			if lastMessageStrings[update.tag] != updateString {
				lastMessages[update.tag] = update.state
				lastMessageStrings[update.tag] = updateString

				if _, ok := nosaveMessages[strings.ToLower((update.tag))]; !ok {
					saveStateOnceTimer.Stop()
					saveStateOnceTimer = time.NewTimer(saveDelayAfterChange)
				}
			}

		case <-saveStateRegularlyTicker.C:
			saveState(lastMessages)

		case <-saveStateOnceTimer.C:
			saveState(lastMessages)
		}
	}
}

// nosaveMessages is a set of message names that you don't save, because they
// contain no configuration that makes sense to preserve across runs of dastard.
var nosaveMessages = map[string]struct{}{
	"channelnames":  {},
	"alive":         {},
	"triggerrate":   {},
	"numberwritten": {},
	"newdastard":    {},
}

// saveState stores server configuration to the standard config file.
func saveState(lastMessages map[string]interface{}) {

	lastMessages["CURRENTTIME"] = time.Now().Format(time.UnixDate)
	// Note that the nosaveMessages don't get into the lastMessages map.
	for k, v := range lastMessages {
		if _, ok := nosaveMessages[strings.ToLower(k)]; !ok {
			viper.Set(k, v)
		}
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
