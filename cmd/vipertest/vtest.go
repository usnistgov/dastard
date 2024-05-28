package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type newdata struct {
	Figgie    string
	NChildren int
}

func main() {
	viper.SetDefault("Verbose", false)

	viper.SetConfigName("testconfig")
	viper.AddConfigPath(filepath.FromSlash("/etc/dastard"))
	HOME, err := os.UserHomeDir()
	if err != nil { // Handle errors reading the config file
		fmt.Printf("Error finding User Home Dir: %s\n", err)
	}
	viper.AddConfigPath(filepath.Join(HOME, ".dastard"))
	viper.AddConfigPath(".")
	err = viper.ReadInConfig() // Find and read the config file
	if err != nil {            // Handle errors reading the config file
		fmt.Printf("Error reading config file: %s \n", err)
	}

	fmt.Printf("Verbose:  %t\n", viper.Get("Verbose"))
	fmt.Printf("Figgie:   %v\n", viper.Get("Figgie"))
	fmt.Printf("sfffd:    %v\n", viper.Get("sdffd"))
	fmt.Printf("cur time: %v\n", viper.Get("currenttime"))

	d := newdata{"black cat", 3}
	viper.Set("newdata", d)
	viper.WriteConfig()
}
