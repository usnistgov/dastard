package main

import (
	"fmt"

	"github.com/spf13/viper"
)

type newdata struct {
	Figgie    string
	NChildren int
}

func main() {
	viper.SetDefault("Verbose", false)

	viper.SetConfigName("testconfig")
	viper.AddConfigPath("/etc/dastard")
	viper.AddConfigPath("$HOME/.config/dastard")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		fmt.Printf("Error reading config file: %s \n", err)
	}

	fmt.Printf("Verbose: %t\n", viper.Get("Verbose"))
	fmt.Printf("Figgie:   %v\n", viper.Get("Figgie"))
	fmt.Printf("sfffd:   %v\n", viper.Get("sdffd"))

	d := newdata{"black cat", 3}
	viper.Set("newdata", d)
	viper.WriteConfig()
}
