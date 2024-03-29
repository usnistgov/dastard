package dastard

// Contain the ClientUpdater object, which publishes JSON-encoded messages
// giving the latest DASTARD state. Most of these messages are saved to
// disk with viper.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/pebbe/zmq4"
	"github.com/spf13/viper"
)

// ClientUpdate carries the messages to be published on the status port.
type ClientUpdate struct {
	tag   string
	state interface{}
}

// nopublishMessages is a set of message names that you don't send to clients, because they
// contain no configuration that makes sense for clients to hear.
var nopublishMessages = map[string]struct{}{
	"CURRENTTIME": {},
	"___1":        {},
	"___2":        {},
	"___3":        {},
	"___4":        {},
	"___5":        {},
}

// nologMessages is a set of message names that you don't log to the terminal, because they
// are too long or too frequent to bother with.
var nologMessages = map[string]struct{}{
	"TRIGGERRATE":     {},
	"CHANNELNAMES":    {},
	"ALIVE":           {},
	"NUMBERWRITTEN":   {},
	"EXTERNALTRIGGER": {},
}

// var messageSerial int

// publish sends to all clients of the status update socket a 2-part message, with
// the `update.tag` as the first part and `message` as the second. The latter should be
// decodable as JSON.
func publish(pubSocket *zmq4.Socket, update ClientUpdate, message []byte) {
	updateType := reflect.TypeOf(update.state).String()
	tag := update.tag
	if _, ok := nopublishMessages[tag]; ok {
		return
	}
	if _, ok := nologMessages[tag]; !ok {
		UpdateLogger.Printf("SEND %v %v\n-> message body: %v\n", tag, updateType, string(message))
	}
	// Send the 2-part message to all subscribers (clients).
	// If there are errors, retry up to `maxSendAttempts` times with a sleep between.
	fullmessage := [][]byte{[]byte(tag), message}
	const maxSendAttempts = 5
	var err error
	for iter := 0; iter < maxSendAttempts; iter++ {
		if _, err = pubSocket.SendMessage(fullmessage); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil {
		fmt.Printf("Could not send a %s message even with %d attempts in client_updater.publish", tag, maxSendAttempts)
		panic(err)
	}
}

var clientMessageChan chan ClientUpdate

func init() {
	clientMessageChan = make(chan ClientUpdate, 10)
}

// RunClientUpdater forwards any message from its input channel to the ZMQ publisher socket
// to publish any information that clients need to know.
func RunClientUpdater(statusport int, abort <-chan struct{}) {
	hostname := fmt.Sprintf("tcp://*:%d", statusport)
	pubSocket, err := zmq4.NewSocket(zmq4.PUB)
	if err != nil {
		panicmsg := fmt.Errorf("could not create client updater port %d\n\terr=%v", statusport, err)
		panic(panicmsg)
	}
	defer pubSocket.Close()
	// pubSocket.SetSndhwm(100)
	if err = pubSocket.Bind(hostname); err != nil {
		panicmsg := fmt.Errorf("could not bind client updater port %d\n\terr=%v", statusport, err)
		panic(panicmsg)
	}

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
		case <-abort:
			return

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
	"channelnames":    {},
	"alive":           {},
	"triggerrate":     {},
	"numberwritten":   {},
	"newdastard":      {},
	"tesmap":          {},
	"externaltrigger": {},
}

// saveState stores server configuration to the standard config file.
func saveState(lastMessages map[string]interface{}) {

	lastMessages["___1"] = "DASTARD configuration file. Written and read by DASTARD."
	lastMessages["___2"] = "Human intervention by experts is permitted but not expected."
	now := time.Now().Format(time.UnixDate)
	lastMessages["CURRENTTIME"] = now
	// Note that the nosaveMessages shouldn't get into the lastMessages map.
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
