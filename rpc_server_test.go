package dastard

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/user"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"gonum.org/v1/gonum/mat"
)

func simpleClient() (*rpc.Client, error) {
	serverAddress := fmt.Sprintf("localhost:%d", Ports.RPC)
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

func TestServer(t *testing.T) {
	client, err := simpleClient()
	defer client.Close()
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

	// Test a basic configuration
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
	if err == nil {
		t.Errorf("expected error on Stopping when there is no active source")
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
	err = client.Call("SourceControl.Start", &sourceName, &okay)
	if err == nil {
		t.Errorf("expected error when starting Source while a source is active")
	}
	dummy := ""
	err = client.Call("SourceControl.SendAllStatus", &dummy, &okay)
	if err != nil {
		t.Error("Error calling SourceControl.SendAllStatus():", err)
	}
	time.Sleep(time.Millisecond * 400)
	sizes := SizeObject{Nsamp: 800, Npre: 200}
	err = client.Call("SourceControl.ConfigurePulseLengths", &sizes, &okay)
	if err != nil {
		t.Logf(err.Error())
		t.Errorf("Error calling SourceControl.ConfigurePulseLengths(%v)", sizes)
	}
	if !okay {
		t.Errorf("SourceControl.ConfigurePulseLengths(%v) returns !okay, want okay", sizes)
	}
	err = client.Call("SourceControl.Stop", sourceName, &okay)
	if err != nil {
		t.Logf(err.Error())
		t.Errorf("Error calling SourceControl.Stop(%s)", sourceName)
	}
	if !okay {
		t.Errorf("SourceControl.Stop(\"%s\") returns !okay, want okay", sourceName)
	}
	err = client.Call("SourceControl.ConfigurePulseLengths", &sizes, &okay)
	if err == nil {
		t.Errorf("Expected error calling SourceControl.ConfigurePulseLengths(%v) when source stopped, saw none", sizes)
	}

	// Configure, start, and stop a triangle server
	tconfig := TriangleSourceConfig{
		Nchan:      4,
		SampleRate: 10000.0,
		Min:        10,
		Max:        1510,
	}
	err = client.Call("SourceControl.ConfigureTriangleSource", &tconfig, &okay)
	if !okay {
		t.Errorf("Error on server with SourceControl.ConfigureTriangleSource()")
	}
	if err != nil {
		t.Errorf("Error calling SourceControl.ConfigureTriangleSource(): %s", err.Error())
	}

	sourceName = "TriangleSource"
	err = client.Call("SourceControl.Start", &sourceName, &okay)
	if err != nil {
		t.Errorf("Error calling SourceControl.Start(%s): %s", sourceName, err.Error())
	}
	if !okay {
		t.Errorf("SourceControl.Start(\"%s\") returns !okay, want okay", sourceName)
	}
	time.Sleep(time.Millisecond * 400)
	rows := 5
	cols := 1000
	projectors := mat.NewDense(rows, cols, make([]float64, rows*cols))
	basis := mat.NewDense(cols, rows, make([]float64, rows*cols))
	projectorsBytes, err := projectors.MarshalBinary()
	if err != nil {
		t.Error(err)
	}
	basisBytes, err := basis.MarshalBinary()
	if err != nil {
		t.Error(err)
	}
	pbo := ProjectorsBasisObject{ProcessorIndex: 0,
		ProjectorsBase64: base64.StdEncoding.EncodeToString(projectorsBytes),
		BasisBase64:      base64.StdEncoding.EncodeToString(basisBytes)}

	err = client.Call("SourceControl.ConfigureProjectorsBasis", &pbo, &okay)
	if err != nil {
		t.Error(err)
	}
	if !okay {
		t.Errorf("SourceControl.ConfigureProjectorsBasis(\"%s\") returns !okay, want okay", sourceName)
	}
	mfo := MixFractionObject{0, 1.0}
	if err := client.Call("SourceControl.ConfigureMixFraction", &mfo, &okay); err == nil {
		t.Error("error on ConfigureMixFraction expected for non-mixable source")
	}
	tstate := FullTriggerState{ChanNumbers: []int{0, 1, 2}}
	if err := client.Call("SourceControl.ConfigureTriggers", &tstate, &okay); err != nil {
		t.Error("error on ConfigureTriggers:", err)
	}
	err = client.Call("SourceControl.Stop", sourceName, &okay)
	if err != nil {
		t.Errorf("Error calling SourceControl.Stop(%s)\n%v", sourceName, err)
	}
	if !okay {
		t.Errorf("SourceControl.Stop(\"%s\") returns !okay, want okay", sourceName)
	}

	// Make sure Nchan = 0 raises error when we try to configure
	simConfig.Nchan = 0
	err = client.Call("SourceControl.ConfigureSimPulseSource", &simConfig, &okay)
	if err == nil {
		t.Errorf("Expected error on server with SourceControl.ConfigureSimPulseSource() when Nchan<1, %t %v", okay, err)
	}
	tconfig.Nchan = 0
	err = client.Call("SourceControl.ConfigureTriangleSource", &tconfig, &okay)
	if err == nil {
		t.Errorf("Expected error on server with SourceControl.ConfigureTriangleSource() when Nchan<1")
	}

	// here test all methods that expect an active source to make sure they error appropriatley
	// otherwise you will get incomprehensible stack traces when they error unexpectedly
	if err := client.Call("SourceControl.ConfigureProjectorsBasis", &pbo, &okay); err == nil {
		t.Error("expected error on ConfigureProjectorsBasiswhen no source is active")
	}
	if err := client.Call("SourceControl.Stop", sourceName, &okay); err == nil {
		t.Errorf("expected error stopping source when no source is active")
	}
	if err := client.Call("SourceControl.ConfigureMixFraction", &mfo, &okay); err == nil {
		t.Error("expected error on ConfigureMixFraction when no source is active")
	}
	if err := client.Call("SourceControl.ConfigureTriggers", &tstate, &okay); err == nil {
		t.Error("expected error on ConfigureTriggers when no source is active")
	}

	// lancero source should fail
	sourceName = "LanceroSource"
	err = client.Call("SourceControl.Start", &sourceName, &okay)
	if err != nil {
		t.Errorf("Error calling SourceControl.Start(%s): %s", sourceName, err.Error())
	}
	if !okay {
		t.Errorf("SourceControl.Start(\"%s\") returns !okay, want okay", sourceName)
	}
}

// verifyConfigFile checks that path/filename exists, and creates the directory
// and file if it doesn't.
func verifyConfigFile(path, filename string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	path = strings.Replace(path, "$HOME", u.HomeDir, 1)

	// Create directory <path>, if needed
	_, err = os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		err = os.MkdirAll(path, 0775)
		if err != nil {
			return err
		}
	}

	// Create an empty file path/filename, if it doesn't exist.
	fullname := fmt.Sprintf("%s/%s", path, filename)
	_, err = os.Stat(fullname)
	if os.IsNotExist(err) {
		f, err := os.OpenFile(fullname, os.O_WRONLY|os.O_CREATE, 0664)
		if err != nil {
			return err
		}
		f.Close()
	}
	return nil
}

// setupViper sets up the viper configuration manager: says where to find config
// files and the filename and suffix. Sets some defaults.
func setupViper() error {
	viper.SetDefault("Verbose", false)

	const path string = "$HOME/.dastard"
	const filename string = "testconfig"
	const suffix string = ".yaml"
	if err := verifyConfigFile(path, filename+suffix); err != nil {
		return err
	}

	viper.SetConfigName(filename)
	viper.AddConfigPath(path)
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		return fmt.Errorf("error reading config file: %s", err)
	}

	// Set up different ports for testing than you'd use otherwise
	setPortnumbers(33000)
	return nil
}

func TestMain(m *testing.M) {
	// Find config file, creating it if needed, and read it.
	if err := setupViper(); err != nil {
		panic(err)
	}

	// call flag.Parse() here if TestMain uses flags
	messageChan := make(chan ClientUpdate)
	go RunClientUpdater(messageChan, Ports.Status)
	go RunRPCServer(messageChan, Ports.RPC)
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
