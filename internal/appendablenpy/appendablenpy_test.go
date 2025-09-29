package appendablenpy

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path"
	"testing"
)


func TestWrite(t *testing.T) {
	// dirname := t.TempDir()
	dirname := "."
	file := path.Join(dirname, "empty.npy") 
	dtype := "[('ch_num', '<u2'), ('timestamp', '<u8'), ('subframe', '<u8'), ('pulse', '<u2', (100,))]"
	fp, err := os.Create(file)
	if err != nil {
		t.Error(err)
	}
	defer fp.Close()
	npy := OpenAppendableNPY(fp, dtype)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Error(err)
	}
	if len(data) != 192 {
		t.Errorf("len(data) is %d, want %d", len(data), 192)
	}
	pythonScript := "./test.py"
	cmd := exec.Command("python", pythonScript, file, "0") 
	// Run the command and capture its output and error
	output, err := cmd.CombinedOutput()

	// Check for execution errors
	if err != nil {
		t.Fatalf("Python script execution failed: %v\nOutput: %s", err, output)
	}

	
	pulse := make([]byte, 2*100)
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint16(55))
	binary.Write(buf, binary.LittleEndian, uint64(99))
	binary.Write(buf, binary.LittleEndian, uint64(1000))
	binary.Write(buf, binary.LittleEndian, pulse)

	b := buf.Bytes()
	npy.Write([][]byte{b, b, b, b, b})
	data, err = os.ReadFile(file)
	if err != nil {
		t.Error(err)
	}
	want := 192 + 218*5
	if len(data) != want {
		t.Errorf("len(data) is %d, want %d", len(data), want)
	}

	cmd = exec.Command("python", pythonScript, file, "5") 

	// Run the command and capture its output and error
	output, err = cmd.CombinedOutput()

	// Check for execution errors
	if err != nil {
		t.Fatalf("Python script execution failed: %v\nOutput: %s", err, output)
	}

	for i := 0; i < 100; i++ {
		npy.Write([][]byte{b, b, b, b, b})
	}
	data, err = os.ReadFile(file)
	if err != nil {
		t.Error(err)
	}
	want = 192 + 218*505
	if len(data) != want {
		t.Errorf("len(data) is %d, want %d", len(data), want)
	}

	cmd = exec.Command("python", pythonScript, file, "505") 

	// Run the command and capture its output and error
	output, err = cmd.CombinedOutput()

	// Check for execution errors
	if err != nil {
		t.Fatalf("Python script execution failed: %v\nOutput: %s", err, output)
	}

	// Assert the expected output or behavior
	// expectedOutput := "Python script executed successfully.\n"
	// if string(output) != expectedOutput {
	// 	t.Errorf("Unexpected Python script output. Expected:\n%sGot:\n%s", expectedOutput, output)
	// }
}