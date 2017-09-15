package main

import (
    "fmt"
    "github.com/usnistgov/dastard/lancero"
)

func main() {
    dev, err := lancero.Open(0)
    fmt.Println(dev)
    fmt.Println(err)
    if err == nil {
        err = dev.Close()
        fmt.Println(dev)
        fmt.Println(err)
    }
}
