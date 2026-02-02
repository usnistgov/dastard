package main

import (
	"fmt"
	"net"
	"time"
)

// From https://jameshfisher.com/2016/11/17/udp-in-go.html
func main() {
	Conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: []byte{127, 0, 0, 1}, Port: 12321, Zone: ""})
	defer Conn.Close()
	for i := 1; i <= 60; i++ {
		Conn.Write(fmt.Appendf(nil, "hello %d", i))
		time.Sleep(time.Second)
	}
}
