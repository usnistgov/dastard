package main

import (
	"fmt"

	"github.com/usnistgov/dastard/lancero"
)

func main() {
	dev, err := lancero.NewLancero(0)
	if err != nil {
		fmt.Println("Could not open the device. Error is:")
		fmt.Println(err)
		return
	}
	defer dev.Close()

	fmt.Println(dev)
}
