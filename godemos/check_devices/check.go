package main

import (
	"fmt"
	"os"
)

func checkNDFB() (results []string) {
	fname := "/dev/ndfb"
	f, err := os.Open(fname)
	if err == nil {
		results = append(results, fname)
		f.Close()
	}
	for i := 0; i < 10; i++ {
		fname := fmt.Sprintf("/dev/ndfb%d", i)
		f, err := os.Open(fname)
		if err == nil {
			results = append(results, fname)
			f.Close()
		}
	}
	return
}

// Check for lancero devices and return a slice of their names.
// Funny handling is required because the old-style name /dev/lancero_user is
// now called /dev/lancero_user0. If both are present, prefer the latter; if
// only one is present, use it.
func checkLancero() (results []string) {
	oldLancero := false
	_, err := os.Open("/dev/lancero_user")
	if err == nil {
		oldLancero = true
	}

	for i := 0; i < 10; i++ {
		fname := fmt.Sprintf("/dev/lancero_user%d", i)
		f, err := os.Open(fname)
		if err == nil {
			results = append(results, fname)
			f.Close()
		}
	}
	if len(results) == 0 && oldLancero {
		results = append(results, "/dev/lancero_user")
	}
	return
}

// checkMisc checks for a few random dev-special files that are likely
// to be present and readable on a non-DAQ computer.
func checkMisc() (results []string) {
	names := []string{"random", "null", "zero", "disk0", "pf"}
	for _, name := range names {
		fname := fmt.Sprintf("/dev/%s", name)
		f, err := os.Open(fname)
		if err == nil {
			results = append(results, fname)
			f.Close()
		}
	}
	return
}

func main() {
	fmt.Println("Here are some device-special files that we can open for reading.")
	fmt.Printf("NDFB Devices:    %s\n", checkNDFB())
	fmt.Printf("Lancero Devices: %s\n", checkLancero())
	fmt.Printf("Misc Devices:    %s\n", checkMisc())
}
