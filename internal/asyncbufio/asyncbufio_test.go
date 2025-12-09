package asyncbufio

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"testing"
	"time"
)

func md5sum(fname string) string {
	f, err := os.Open(fname)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestWrite(t *testing.T) {
	f, err := os.CreateTemp("", "example")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(f.Name()) // clean up

	w := NewWriter(f, 100, time.Second)
	for i := range 100 {
		sometext := fmt.Appendf(nil, "Line of text %3d\n", i)
		w.Write(sometext)
		if i%25 == 19 {
			w.Flush()
		}
	}
	w.Write([]byte("Last line\n"))
	w.Close()

	// Verify exact file contents
	actual := md5sum(f.Name())
	expected := "49c3d3dc6d2929a997016c9509010333"
	if actual != expected {
		t.Errorf("example file md5=%s, want %s", actual, expected)
	}

	// Tricky way to test for an expected panic:
	defer func() { recover() }()
	w.Flush()
	t.Errorf("asyncbufio.Writer.Flush() after .Close() did not panic")
}

func TestCloseTwice(t *testing.T) {
	f, err := os.CreateTemp("", "example")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(f.Name()) // clean up

	w := NewWriter(f, 100, time.Second)
	w.Close()

	// Tricky way to test for an expected panic:
	defer func() { recover() }()
	w.Close()
	t.Errorf("asyncbufio.Writer.Flush() after .Close() did not panic")
}
