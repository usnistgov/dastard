package main

import (
    "fmt"
    "os"
    "os/signal"
    "time"
)

// Catch ctrl-C (or any other interrupts sent to the program).
// Use a timer to show that the program is alive.

func main() {
    fmt.Println("This program will catch interrupts.")
    catcher := make(chan os.Signal, 1)
    signal.Notify(catcher, os.Interrupt)

    // Now a ticker to show aliveness.
    ticker := time.NewTicker(1*time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            fmt.Println("Tick")
        case sig := <-catcher:
            fmt.Println("Caught signal", sig, "which we would handle here.")
            return
        }
    }

}
