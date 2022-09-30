package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/usnistgov/dastard/packets"
	"net"
	"strconv"
	"strings"
)

func probe(npack int, endpoint string) error {
	address, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return err
	}
	ServerConn, _ := net.ListenUDP("udp", address)
	defer ServerConn.Close()
	buf := make([]byte, 8192)
	for i := 0; i < npack; i++ {
		if _, _, err := ServerConn.ReadFromUDP(buf); err != nil {
			return err
		}
		if pack, err := packets.ReadPacket(bytes.NewReader(buf)); err != nil {
			return err
		} else {
			nchan, chan0 := pack.ChannelInfo()
			fmt.Printf("%s with %d frames for %d channels [%d-%d]\n", pack.String(),
				pack.Frames(), nchan, chan0, chan0+nchan-1)
		}
		// fmt.Println("Received ", string(buf[0:n]), " from ", addr)
	}
	return nil
}

func main() {
	var npack int
	var port int
	host := "localhost"
	const default_port = 4000
	flag.IntVar(&npack, "n", 10, "Number of packets to dump")
	flag.IntVar(&port, "port", default_port, "Port to monitor")
	flag.IntVar(&port, "p", default_port, "Port to monitor (shorthand)")

	flag.Usage = func() {
		fmt.Println("udpdump, for dumping the first N packets' headers")
		fmt.Println("Usage: udpdump [flags] host[:port]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		host = flag.Arg(0)

		// If host ends in :portnum, split that off and update the port value
		if a, b, hascolon := strings.Cut(host, ":"); hascolon {
			host = a
			if attachedport, err := strconv.Atoi(b); err != nil {
				fmt.Printf("Cannot convert port '%s' to integer\n", b)
				return
			} else {
				if port != default_port && port != attachedport {
					fmt.Printf("Cannot use -p argument and a conflicting host:port port\n")
					return
				}
				port = attachedport
			}
		}
	}

	endpoint := fmt.Sprintf("%s:%4.4d", host, port)
	fmt.Printf("Probing %s for first %d packets\n", endpoint, npack)
	if err := probe(npack, endpoint); err != nil {
		fmt.Printf("error: %v\n", err)
	}
}
