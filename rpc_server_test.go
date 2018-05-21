package dastard

import (
	"fmt"
	"log"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"testing"
	"time"
)

func simpleClient() (*rpc.Client, error) {
	serverAddress := fmt.Sprintf("localhost:%d", PortRPC)
	retries := 5
	wait := 10 * time.Millisecond
	tries := 1
	for {
		// One command to dial AND set up jsonrpc client:
		client, err := jsonrpc.Dial("tcp", serverAddress)
		tries++
		if err == nil || tries > retries {
			return client, err
		}
		time.Sleep(wait)
		wait = wait * 2
	}
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
		err = client.Call("DastardControl.Multiply", args, &reply)
		if err != nil {
			t.Errorf("DastardControl.Multiply error on call: %s", err.Error())
		}
		if reply != args.A*args.B {
			t.Errorf("DastardControl.Multiply: %d * %d = %d, want %d\n", args.A, args.B, reply, args.A*args.B)
		}
	}

	// Test a basic start and stop
	var okay bool
	simConfig := SimPulseSourceConfig{
		Nchan: 4, SampleRate: 10000.0, Pedestal: 3000.0,
		Amplitude: 10000., Nsamp: 1000,
	}
	err = client.Call("DastardControl.ConfigureSimPulseSource", &simConfig, &okay)
	if !okay {
		t.Errorf("Error on server with DastardControl.ConfigureSimPulseSource()")
	}
	if err != nil {
		t.Errorf("Error calling DastardControl.ConfigureSimPulseSource(): %s", err.Error())
	}

	// Try to start and stop with a wrong name
	sourceName := "harrypotter"
	err = client.Call("DastardControl.Start", &sourceName, &okay)
	if err == nil {
		t.Errorf("Expected error calling DastardControl.Start(\"%s\") with wrong name, saw none", sourceName)
	}
	err = client.Call("DastardControl.Stop", sourceName, &okay)
	if err != nil {
		t.Logf(err.Error())
		t.Errorf("Error calling DastardControl.Stop(%s)", sourceName)
	}
	if okay {
		t.Errorf("DastardControl.Stop(\"%s\") returns okay, want !okay", sourceName)
	}

	// Try to start and stop with a sensible name
	sourceName = "SimPulseSource"
	err = client.Call("DastardControl.Start", &sourceName, &okay)
	if err != nil {
		t.Errorf("Error calling DastardControl.Start(%s): %s", sourceName, err.Error())
	}
	if !okay {
		t.Errorf("DastardControl.Start(\"%s\") returns !okay, want okay", sourceName)
	}
	time.Sleep(time.Millisecond * 400)
	sizes := SizeObject{Nsamp: 800, Npre: 200}
	err = client.Call("DastardControl.ConfigurePulseLengths", &sizes, &okay)
	if err != nil {
		t.Logf(err.Error())
		t.Errorf("Error calling DastardControl.ConfigurePulseLengths(%v)", sizes)
	}
	if !okay {
		t.Errorf("DastardControl.ConfigurePulseLengths(%v) returns !okay, want okay", sizes)
	}
	err = client.Call("DastardControl.Stop", sourceName, &okay)
	if err != nil {
		t.Logf(err.Error())
		t.Errorf("Error calling DastardControl.Stop(%s)", sourceName)
	}
	if !okay {
		t.Errorf("DastardControl.Stop(\"%s\") returns !okay, want okay", sourceName)
	}
	err = client.Call("DastardControl.ConfigurePulseLengths", &sizes, &okay)
	if err == nil {
		t.Errorf("Expected error calling DastardControl.ConfigurePulseLengths(%v) when source stopped, saw none", sizes)
	}

	// Configure, start, and stop a triangle server
	tconfig := TriangleSourceConfig{
		Nchan:      4,
		SampleRate: 10000.0,
		Min:        10,
		Max:        1510,
	}
	err = client.Call("DastardControl.ConfigureTriangleSource", &tconfig, &okay)
	if !okay {
		t.Errorf("Error on server with DastardControl.ConfigureTriangleSource()")
	}
	if err != nil {
		t.Errorf("Error calling DastardControl.ConfigureTriangleSource(): %s", err.Error())
	}

	sourceName = "TriangleSource"
	err = client.Call("DastardControl.Start", &sourceName, &okay)
	if err != nil {
		t.Errorf("Error calling DastardControl.Start(%s): %s", sourceName, err.Error())
	}
	if !okay {
		t.Errorf("DastardControl.Start(\"%s\") returns !okay, want okay", sourceName)
	}
	time.Sleep(time.Millisecond * 400)
	t.Log("Calling DastardControl.Stop")
	err = client.Call("DastardControl.Stop", sourceName, &okay)
	if err != nil {
		t.Logf(err.Error())
		t.Errorf("Error calling DastardControl.Stop(%s)", sourceName)
	}
	if !okay {
		t.Errorf("DastardControl.Stop(\"%s\") returns !okay, want okay", sourceName)
	}

	// Make sure Nchan = 0 raises error when we try to configure
	simConfig.Nchan = 0
	err = client.Call("DastardControl.ConfigureSimPulseSource", &simConfig, &okay)
	if err == nil {
		t.Errorf("Expected error on server with DastardControl.ConfigureSimPulseSource() when Nchan<1, %t %v", okay, err)
	}
	tconfig.Nchan = 0
	err = client.Call("DastardControl.ConfigureTriangleSource", &tconfig, &okay)
	if err == nil {
		t.Errorf("Expected error on server with DastardControl.ConfigureTriangleSource() when Nchan<1")
	}

	client.Close()
	t.Log("Done with TestOne")
}

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	messageChan := make(chan ClientUpdate)
	go RunClientUpdater(messageChan, PortStatus)
	go RunRPCServer(messageChan, PortRPC)
	// set log to write to a file
	f, err := os.Create("dastardtestlogfile")
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	// run tests
	os.Exit(m.Run())
}
