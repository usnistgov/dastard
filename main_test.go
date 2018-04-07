package main

import (
	"fmt"
	"net/rpc"
	"net/rpc/jsonrpc"
	"testing"
)

func simpleClient() (*rpc.Client, error) {
	serverAddress := fmt.Sprintf("localhost:%d", PortRPC)

	// One command to dial AND set up jsonrpc client:
	return jsonrpc.Dial("tcp", serverAddress)
}

func TestOne(t *testing.T) {
	go main()

	client, err := simpleClient()
	if err != nil {
		t.Errorf("Could not connect simpleClient() to RPC server")
	}

	// Test a basic start and stop
	var remoteerr error
	simConfig := SimPulseSourceConfig{
		Nchan: 4, Rate: 1.0, Pedestal: 3000.0,
		Amplitude: 10000., Nsamp: 1000,
	}
	err = client.Call("SourceControl.ConfigureSimPulseSource", &simConfig, &remoteerr)
	if remoteerr != nil {
		t.Errorf("Error on server with SourceControl.ConfigureSimPulseSource()")
	}
	if err != nil {
		t.Errorf("Error calling SourceControl.ConfigureSimPulseSource(): %s", err.Error())
	}

	// sourceName := "petunia"
	// err = client.Call("SourceControl.Start", &sourceName, &remoteerr)
	// if err != nil {
	// 	t.Errorf("Error calling SourceControl.Start(%s): %s", sourceName, err.Error())
	// }
	// if remoteerr != nil {
	// 	t.Errorf("Error on server with SourceControl.Start(%s)", sourceName)
	// }
	// err = client.Call("SourceControl.Stop", sourceName, &remoteerr)
	// // if err != nil {
	// // 	fmt.Printf(err.Error())
	// // 	t.Errorf("Error calling SourceControl.Stop(%s)", sourceName)
	// // }
	// if remoteerr != nil {
	// 	t.Errorf("Error on server with SourceControl.Stop(%s)", sourceName)
	// }

	// Test the silly multiply feature
	// type Args struct {
	// 	A, B int
	// }
	// args := &Args{33, 0}
	// var reply int
	// for b := 10; b < 14; b++ {
	// 	args.B = b
	// 	err = client.Call("SourceControl.Multiply", args, &reply)
	// 	if err != nil {
	// 		t.Errorf("SourceControl.Multiply error on call: %s", err.Error())
	// 	}
	// 	if reply != args.A*args.B {
	// 		t.Errorf("SourceControl.Multiply: %d * %d = %d, want %d\n", args.A, args.B, reply, args.A*args.B)
	// 	}
	// }
}
