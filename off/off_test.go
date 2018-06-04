package off

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"gonum.org/v1/gonum/mat"
	// "labix.org/v2/mgo/bson"
	"encoding/base64"
)

func matPrint(X mat.Matrix, t *testing.T) {
	fa := mat.Formatted(X, mat.Prefix(""), mat.Squeeze())
	t.Logf("%v\n", fa)
	fmt.Println(fa)
}

func float64SliceToByte(fs []float64) []byte {
	var buf bytes.Buffer
	for _, f := range fs {
		err := binary.Write(&buf, binary.BigEndian, f)
		if err != nil {
			fmt.Println("binary.Write failed:", err)
		}
	}
	return buf.Bytes()
}

type SliceJSON struct {
	Slice []float64
	Cols  int
	Rows  int
}
type SliceJSONBase64 struct {
	SliceBase64 string
	Cols        int
	Rows        int
}
type OFFHeaderExample struct {
	MatMarshalBase64  string          `json:"1"`
	JSONMarshalBase64 SliceJSONBase64 `json:"2"`
	JSONMarshal       SliceJSON       `json:"3"`
}

func TestOff(t *testing.T) {

	// assign the projectors and basis
	nbases := 3
	nsamples := 4
	projectors := mat.NewDense(nbases, nsamples,
		[]float64{1.124, 0, 1.124, 0,
			0, 1, 0, 0,
			0, 0, 1, 0})
	// basis := mat.NewDense(nsamples, nbases,
	// 	[]float64{1, 0, 0,
	// 		0, 1, 0,
	// 		0, 0, 1,
	// 		0, 0, 0})

	r, c := projectors.Dims()
	p := SliceJSON{Slice: projectors.RawMatrix().Data, Rows: r, Cols: c}
	b, _ := projectors.MarshalBinary()
	h := OFFHeaderExample{MatMarshalBase64: base64.StdEncoding.EncodeToString(b), JSONMarshal: p,
		JSONMarshalBase64: SliceJSONBase64{Rows: r, Cols: c,
			SliceBase64: base64.StdEncoding.EncodeToString(float64SliceToByte(projectors.RawMatrix().Data))}}
	bytes, _ := json.MarshalIndent(h, "", "    ")
	fmt.Println(string(bytes))
	f, _ := os.Create("projectorbinary")
	f.Write(bytes)
	f.Close()
	raw, _ := ioutil.ReadFile("projectorbinary")
	hr := OFFHeaderExample{}
	json.Unmarshal(raw, &hr)
	var pr mat.Dense
	b, _ = base64.StdEncoding.DecodeString(hr.MatMarshalBase64)
	pr.UnmarshalBinary(b)
	matPrint(&pr, t)

}
