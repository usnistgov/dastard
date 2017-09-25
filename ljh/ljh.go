// Package ljh provides classes that read or write from the LJH x-ray pulse
// data file format.
package ljh

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// VersionCode enumerates the LJH file version numbers.
type VersionCode int

// Enumeration for the LJH file version numbers
const (
	VersionInvalid VersionCode = iota
	Version2_1
	Version2_2
)

// Reader is the interface for reading an LJH file
type Reader struct {
	ChanNum         int
	Presamples      int
	Samples         int
	VersionNumber   VersionCode
	WordSize        int
	Timebase        float64
	TimestampOffset float64

	recordLength int
	headerLength int
	file         *os.File
}

// OpenReader returns an active LJH file reader, or an error.
func OpenReader(fileName string) (r *Reader, err error) {
	f, err := os.Open(fileName)
	if err != nil {
		return
	}

	r = &Reader{file: f}
	defer r.Close()
	err = r.parseHeader()
	if err != nil {
		return
	}
	return r, nil
}

// Close closes the LJH file reader.
func (r *Reader) Close() error {
	return r.file.Close()
}

func (r *Reader) setVersionNumber(line string) error {
	s := strings.TrimPrefix(line, "Save File Format Version: ")
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return fmt.Errorf("LJH file '%s': could not parse version number '%s'",
			r.file.Name(), s)
	}
	if parts[0] != "2" {
		return fmt.Errorf("LJH file '%s' version number '%s' was not version 2",
			r.file.Name(), s)
	}
	if parts[1] == "1" && parts[2] == "1" {
		r.VersionNumber = Version2_1
		r.recordLength = 6
		return nil
	}
	if parts[1] == "2" {
		r.VersionNumber = Version2_2
		r.recordLength = 16
		return nil
	}
	return fmt.Errorf("LJH file '%s': could not parse version number '%s' to valid value",
		r.file.Name(), s)
}

func extract(line, pattern string, i *int) bool {
	n, err := fmt.Sscanf(line, pattern, i)
	return n >= 1 && err != nil
}

func extractFloat(line, pattern string, f *float64) bool {
	n, err := fmt.Sscanf(line, pattern, f)
	return n >= 1 && err != nil
}

func (r *Reader) parseHeader() error {
	scanner := bufio.NewScanner(r.file)
	lnum := 0
	textLength := 0
	endHeaderTag := "#End of Header"
header:
	for scanner.Scan() {
		line := scanner.Text()
		textLength += len(line)
		switch {
		case lnum == 0:
			firstLine := "#LJH Memorial File Format"
			if line != firstLine {
				return fmt.Errorf("File must begin with '%s'", firstLine)
			}
		case strings.Contains(line, "Save File Format Version:"):
			if err := r.setVersionNumber(line); err != nil {
				return err
			}

		case line == endHeaderTag:
			break header

		case extract(line, "Digitized Word Size in Bytes: %d", &r.WordSize):
		case extract(line, "Presamples: %d", &r.Presamples):
		case extract(line, "Total Samples: %d", &r.Samples):
		case extract(line, "Channel: %d", &r.ChanNum):
		case extract(line, "Channel: %d", &r.ChanNum):
		case extractFloat(line, "Timestamp offset (s): %f", &r.TimestampOffset):
		case extractFloat(line, "Timebase: %f", &r.Timebase):

		}
		lnum++
	}
	r.recordLength += r.WordSize * r.Samples

	// To find the header length is a big pain, because the bufio Reader and Scanner
	// outsmart us by not reporting the \r and/or \n line delimeters to us. We must
	// re-find the header-end tag and consume any \r and \n that follow it.
	b := make([]byte, 1024)
	r.file.ReadAt(b, int64(textLength))
	fmt.Printf("read in %d bytes\n", len(b))
	idx := strings.Index(string(b), endHeaderTag)
	if idx < 0 {
		return fmt.Errorf("Could not find '%s' in LJH file", endHeaderTag)
	}
	idx += len(endHeaderTag)
	// Consume all leading \r and/or \n bytes.
	for _, by := range b[idx:] {
		if by == '\n' || by == '\r' {
			idx++
		} else {
			break
		}
	}
	r.headerLength = textLength + idx
	r.file.Seek(int64(r.headerLength), 0) // rewind to end of header

	fmt.Printf("Parsed %d lines in the header and %d bytes.\n", lnum, textLength)
	fmt.Printf("%v\n", r)
	return nil
}
