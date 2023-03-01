package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"runtime/pprof"
	"strings"

	"internal/mysql"

	"github.com/spf13/viper"
	"github.com/usnistgov/dastard"
	"gopkg.in/natefinch/lumberjack.v2"
)

var githash = "githash not computed"
var buildDate = "build date not computed"

// makeFileExist checks that dir/filename exists, and creates the directory
// and file if it doesn't.
func makeFileExist(dir, filename string) (string, error) {
	// Replace 1 instance of "$HOME" in the path with the actual home directory.
	if strings.Contains(dir, "$HOME") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = strings.Replace(dir, "$HOME", home, 1)
	}

	// Create directory <path>, if needed
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		err2 := os.MkdirAll(dir, 0775)
		if err2 != nil {
			return "", err2
		}
	}

	// Create an empty file path/filename, if it doesn't exist.
	fullname := path.Join(dir, filename)
	_, err := os.Stat(fullname)
	if os.IsNotExist(err) {
		f, err2 := os.OpenFile(fullname, os.O_WRONLY|os.O_CREATE, 0664)
		if err2 != nil {
			return "", err2
		}
		f.Close()
	}
	return fullname, nil
}

// setupViper sets up the viper configuration manager: says where to find config
// files and the filename and suffix. Sets some defaults.
func setupViper() error {
	viper.SetDefault("Verbose", false)

	const path string = "$HOME/.dastard"
	const filename string = "config"
	const suffix string = ".yaml"
	if _, err := makeFileExist(path, filename+suffix); err != nil {
		return err
	}

	viper.SetConfigName(filename)
	viper.AddConfigPath("/etc/dastard")
	viper.AddConfigPath(path)
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		return fmt.Errorf("error reading config file: %s", err)
	}
	return nil
}

func startProblemLogger(pfname string) *log.Logger {
	probFile, err := os.OpenFile(pfname, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		msg := fmt.Sprintf("Could not open log file '%s'", pfname)
		panic(msg)
	}
	probLogger := log.New(probFile, "", log.LstdFlags)
	probLogger.SetOutput(&lumberjack.Logger{
		Filename:   pfname,
		MaxSize:    10,   // megabytes after which new file is created
		MaxBackups: 5,    // number of backups
		MaxAge:     365,  // days
		Compress:   true, // whether to gzip the backups
	})
	return probLogger
}

func main() {
	buildDate = strings.Replace(buildDate, ".", " ", -1) // workaround for Make problems
	dastard.Build.Date = buildDate
	dastard.Build.Githash = githash
	dastard.Build.Summary = fmt.Sprintf("DASTARD version %s (git commit %s)", dastard.Build.Version, githash)

	printVersion := flag.Bool("version", false, "print version and quit")
	cpuprofile := flag.String("cpuprofile", "", "write CPU profile to this file")
	memprofile := flag.String("memprofile", "", "write memory profile to this file")
	pingmysql := flag.Bool("ping-mysql", false, "connect to MySQL server, test with a ping, and quit")
	flag.Parse()
	quitImmediately := false

	if *printVersion {
		fmt.Printf("This is DASTARD version %s\n", dastard.Build.Version)
		fmt.Printf("Git commit hash: %s\n", githash)
		fmt.Printf("Build date+time: %s\n", buildDate)
		fmt.Printf("Running on %d CPUs.\n", runtime.NumCPU())
		quitImmediately = true
	} else {
		fmt.Printf("\nThis is %s\n", dastard.Build.Summary)
	}

	// Pinging the MySQL server prints messages and ends the program.
	if *pingmysql {
		mysql.PingMySQLServer()
		quitImmediately = true
	}

	if quitImmediately {
		os.Exit(0)
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Start logging problems to a log file.
	logname, err := makeFileExist("$HOME/.dastard/logs", "problems.log")
	if err != nil {
		panic(err)
	}
	dastard.ProblemLogger = startProblemLogger(logname)
	fmt.Printf("Logging problems to %s\n\n", logname)

	// Find config file, creating it if needed, and read it.
	if err := setupViper(); err != nil {
		panic(err)
	}

	abort := make(chan struct{})
	mysql.StartMySQLConnection(abort)
	go dastard.RunClientUpdater(dastard.Ports.Status, abort)

	dastard.RunRPCServer(dastard.Ports.RPC, true)
	close(abort)
	writeMemoryProfile(memprofile)
}

// writeMemoryProfile writes the memory use profile to the indicated file.
// If `memprofile` points to an empty string, do not write.
func writeMemoryProfile(memprofile *string) {
	if *memprofile == "" {
		return
	}

	f, err := os.Create(*memprofile)
	if err != nil {
		log.Fatal("could not create memory profile: ", err)
	}
	defer f.Close() // error handling omitted for example
	runtime.GC()    // get up-to-date statistics
	if err := pprof.WriteHeapProfile(f); err != nil {
		log.Fatal("could not write memory profile: ", err)
	}
}
