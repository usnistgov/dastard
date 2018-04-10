package main

import (
	"flag"
	"fmt"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"testing"
	"time"
)

func simpleClient() (*rpc.Client, error) {
	serverAddress := fmt.Sprintf("localhost:%d", PortRPC)

	// One command to dial AND set up jsonrpc client:
	return jsonrpc.Dial("tcp", serverAddress)
}

func TestOne(t *testing.T) {
	client, err := simpleClient()
	if err != nil {
		t.Fatalf("Could not connect simpleClient() to RPC server")
	}

	// Test the silly multiply feature
	type Args struct {
		A, B int
	}
	args := &Args{33, 0}
	var reply int
	for b := 10; b < 11; b++ {
		args.B = b
		err = client.Call("SourceControl.Multiply", args, &reply)
		if err != nil {
			t.Errorf("SourceControl.Multiply error on call: %s", err.Error())
		}
		if reply != args.A*args.B {
			t.Errorf("SourceControl.Multiply: %d * %d = %d, want %d\n", args.A, args.B, reply, args.A*args.B)
		}
	}

	// Test a basic start and stop
	var okay bool
	simConfig := SimPulseSourceConfig{
		Nchan: 4, SampleRate: 10000.0, Pedestal: 3000.0,
		Amplitude: 10000., Nsamp: 1000,
	}
	err = client.Call("SourceControl.ConfigureSimPulseSource", &simConfig, &okay)
	if !okay {
		t.Errorf("Error on server with SourceControl.ConfigureSimPulseSource()")
	}
	if err != nil {
		t.Errorf("Error calling SourceControl.ConfigureSimPulseSource(): %s", err.Error())
	}

	// Try to start and stop with a wrong name
	sourceName := "harrypotter"
	err = client.Call("SourceControl.Start", &sourceName, &okay)
	if err == nil {
		t.Errorf("Expected error calling SourceControl.Start(\"%s\") with wrong name, saw none", sourceName)
	}
	err = client.Call("SourceControl.Stop", sourceName, &okay)
	if err != nil {
		fmt.Printf(err.Error())
		t.Errorf("Error calling SourceControl.Stop(%s)", sourceName)
	}
	if okay {
		t.Errorf("SourceControl.Stop(\"%s\") returns okay, want !okay", sourceName)
	}

	// Try to start and stop with a sensible name
	sourceName = "SimPulseSource"
	err = client.Call("SourceControl.Start", &sourceName, &okay)
	if err != nil {
		t.Errorf("Error calling SourceControl.Start(%s): %s", sourceName, err.Error())
	}
	if !okay {
		t.Errorf("SourceControl.Start(\"%s\") returns !okay, want okay", sourceName)
	}
	time.Sleep(time.Second * 2)
	fmt.Println("Calling SourceControl.Stop")
	err = client.Call("SourceControl.Stop", sourceName, &okay)
	if err != nil {
		fmt.Printf(err.Error())
		t.Errorf("Error calling SourceControl.Stop(%s)", sourceName)
	}
	if !okay {
		t.Errorf("SourceControl.Stop(\"%s\") returns !okay, want okay", sourceName)
	}

	client.Close()
	fmt.Println("Done with TestOne")
}

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	flag.Parse()
	go main()
	os.Exit(m.Run())
}
