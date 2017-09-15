package main

import (
    "fmt"
    "time"
)

func main() {
    ticker := time.NewTicker(1*time.Second)
    defer ticker.Stop()

    cancel := make(chan struct{})
    go func () {
        for {
            select {
            case <-cancel:
                return
            case t := <- ticker.C:
                fmt.Println(t)
            }
        }
    }()

    // Let the ticker run for 3.2 seconds, cancel it, and wait to be sure its
    // messages have stopped.
    time.Sleep(3200*time.Millisecond)
    fmt.Println("Killing the ticker")
    close(cancel)
    time.Sleep(4*time.Second)
}
