package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/usnistgov/dastard"
)

// verifyConfigFile checks that path/filename exists, and creates the directory
// and file if it doesn't.
func verifyConfigFile(path, filename string) error {
	path = strings.Replace(path, "$HOME", os.Getenv("HOME"), 1)

	// Create directory <path>, if needed
	_, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		err = os.MkdirAll(path, 0775)
		if err != nil {
			return err
		}
	}

	// Create an empty file path/filename, if it doesn't exist.
	fullname := fmt.Sprintf("%s/%s", path, filename)
	_, err = os.Stat(fullname)
	if os.IsNotExist(err) {
		f, err := os.OpenFile(fullname, os.O_WRONLY|os.O_CREATE, 0664)
		if err != nil {
			return err
		}
		f.Close()
	}
	return nil
}

func setupViper() error {
	viper.SetDefault("Verbose", false)

	const path string = "$HOME/.config/dastard"
	const filename string = "xtestconfig"
	const suffix string = ".yaml"
	if err := verifyConfigFile(path, filename+suffix); err != nil {
		return err
	}

	viper.SetConfigName(filename)
	viper.AddConfigPath("/etc/dastard")
	viper.AddConfigPath(path)
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		return fmt.Errorf("error reading config file: %s", err)
	}
	return nil
}

func main() {
	// Find config file, creating it if needed, and read it.
	if err := setupViper(); err != nil {
		panic(err)
	}

	messageChan := make(chan dastard.ClientUpdate)
	go dastard.RunClientUpdater(messageChan, dastard.PortStatus)
	dastard.RunRPCServer(messageChan, dastard.PortRPC)
}
