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
	fmt.Printf("Probing %s for the first %d packets received...\n", endpoint, npack)
	address, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return err
	}
	ServerConn, _ := net.ListenUDP("udp", address)
	defer ServerConn.Close()

	buf := make([]byte, 8192)
	for range npack {
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
	}
	return nil
}

func main() {
	var npack int
	var port int
	const default_host = "localhost"
	const default_port = 4000
	host := default_host
	flag.IntVar(&npack, "n", 10, "Number of packets to dump")
	flag.IntVar(&port, "port", default_port, "Port to monitor")
	flag.IntVar(&port, "p", default_port, "Port to monitor (shorthand)")

	flag.Usage = func() {
		fmt.Printf("udpdump, for dumping the first N packet headers, by default those from localhost:%d\n",
			default_port)
		fmt.Println("Usage: udpdump [flags] [host][:port]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		host = flag.Arg(0)

		// If host ends in :portnum, split that off and update the port value
		if pieces := strings.Split(host, ":"); len(pieces) > 1 {
			if len(pieces) > 2 {
				fmt.Printf("Cannot parse host '%s' with %d colon separators\n", host, len(pieces)-1)
				return
			}
			attachedport, err := strconv.Atoi(pieces[1])
			if err != nil {
				fmt.Printf("Cannot convert port '%s' to integer\n", pieces[1])
				return
			}
			if port != default_port && port != attachedport {
				fmt.Printf("Cannot use -p argument and a conflicting host:port pair\n")
				return
			}
			if len(pieces[0]) == 0 {
				host = default_host
			} else {
				host = pieces[0]
			}
			port = attachedport
		}
	}

	endpoint := fmt.Sprintf("%s:%4.4d", host, port)
	if err := probe(npack, endpoint); err != nil {
		fmt.Printf("error: %v\n", err)
	}
}
