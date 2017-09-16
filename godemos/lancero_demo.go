package main

import (
    "fmt"
    "github.com/usnistgov/dastard/lancero"
)

func main() {
  dev, err := lancero.NewLancero(0)
  defer dev.Close()

  fmt.Println(dev)
  fmt.Println(err)
}
