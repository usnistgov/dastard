package dastard

import (
	"reflect"
	"testing"
	"time"
)

// TestSegment checks that DataSegment works as expected
func TestSegment(t *testing.T) {
	seg0 := new(DataSegment)
	if len(seg0.rawData) > 0 {
		t.Errorf(`new(DataSegment) length = %d, want 0`, len(seg0.rawData))
	}

	for _, n := range []int{0, 1, 5, 100} {
		segN := &DataSegment{rawData: make([]RawType, n)}
		if len(segN.rawData) != n {
			t.Errorf("new(DataSegment) length = %d, want %d", len(segN.rawData), n)
		}
	}
}

// TestStream checks that DataStream works as expected
func TestStream(t *testing.T) {
	str0 := new(DataStream)
	if len(str0.rawData) > 0 {
		t.Errorf(`new(DataStream) length = %d, want 0`, len(str0.rawData))
	}

	for _, n := range []int{0, 1, 10, 100} {
		strN := &DataStream{rawData: make([]RawType, n)}
		if len(strN.rawData) != n {
			t.Errorf("new(DataStream) length = %d, want %d", len(strN.rawData), n)
		}
	}

	// Test DataStream.AppendSegment(DataSegment)
	ftime := 5 * time.Second
	dA := []RawType{0, 1, 2, 3, 4, 5, 6}
	dB := []RawType{10, 7, 8, 9, 10}
	dC := append(dA, dB...)
	tA := time.Now()
	tB := tA.Add(ftime * time.Duration(len(dA)))
	strA := &DataStream{rawData: dA, firstFramenum: 0, firstTime: tA, framePeriod: ftime}
	strB := &DataSegment{rawData: dB, firstFramenum: int64(len(dA)), firstTime: tB, framePeriod: ftime}

	strA.AppendSegment(strB)
	if len(strA.rawData) != len(dC) {
		t.Errorf("DataStream.AppendSegment result was length %d, want %d", len(strA.rawData), len(dC))
	}
	if !reflect.DeepEqual(strA.rawData, dC) {
		t.Errorf("DataStream.AppendSegment result was %v, want %v", strA.rawData, dC)
	}
	expectf1 := strB.firstFramenum - int64(len(dA))
	if strA.firstFramenum != expectf1 {
		t.Errorf("DataStream.AppendSegment firstFramenum = %d, want %d", strA.firstFramenum,
			expectf1)
	}
	if strA.firstTime != tA {
		t.Errorf("DataStream.AppendSegment firstTime = %v, want %v", strA.firstTime, tA)
	}

	// Test DataStream.TrimKeepingN(int)
	trimN := []int{100, 11, 10, 9, 8, 5, 8, 5, 2, 1, 0}
	for _, N := range trimN {
		trimmedLength := len(strA.rawData)
		if trimmedLength > N {
			trimmedLength = N
		}

		strA.TrimKeepingN(N)
		expectedData := dC[len(dC)-trimmedLength:]
		if len(strA.rawData) != len(expectedData) {
			t.Errorf("DataStream.TrimKeepingN result was length %d, want %d", len(strA.rawData), len(expectedData))
		}
		if !reflect.DeepEqual(strA.rawData, expectedData) {
			t.Errorf("DataStream.TrimKeepingN result was %v, want %v", strA.rawData, expectedData)
		}
		if cap(strA.rawData) < len(dC) {
			t.Errorf("DataStream.TrimKeepingN left cap(rawData)=%d, want at least %d", cap(strA.rawData), len(dC))
		}
		expectf1 := int64(len(dC) - len(strA.rawData))
		if strA.firstFramenum != expectf1 {
			t.Errorf("DataStream.TrimKeepingN firstFramenum = %d, want %d", strA.firstFramenum,
				expectf1)
		}
		expectt1 := tA.Add(time.Duration(len(dC)-len(strA.rawData)) * ftime)
		if strA.firstTime != expectt1 {
			t.Errorf("DataStream.TrimKeepingN firstTime = %v, want %v", strA.firstTime, expectt1)
		}
	}
}

// TestStreamGap checks that DataStream appends as expected, even when time/frame number
// aren't consistent
func TestStreamGap(t *testing.T) {
	ftime := 5 * time.Second
	dA := []RawType{6, 4, 2, 5, 1, 0}
	dB := []RawType{10, 7, 8, 9, 10}
	dC := append(dA, dB...)
	gap := 10
	fA := int64(100)
	fB := int64(len(dA)+gap) + fA
	tA := time.Now()
	tB := tA.Add(ftime * time.Duration(len(dA)+gap))
	strA := &DataStream{rawData: dA, firstFramenum: fA, firstTime: tA, framePeriod: ftime}
	strB := &DataSegment{rawData: dB, firstFramenum: fB, firstTime: tB, framePeriod: ftime}

	strA.AppendSegment(strB)
	if len(strA.rawData) != len(dC) {
		t.Errorf("DataStream.AppendSegment result was length %d, want %d", len(strA.rawData), len(dC))
	}
	if !reflect.DeepEqual(strA.rawData, dC) {
		t.Errorf("DataStream.AppendSegment result was %v, want %v", strA.rawData, dC)
	}
	expectf1 := fA + int64(gap)
	if strA.firstFramenum != expectf1 {
		t.Errorf("DataStream.AppendSegment firstFramenum = %d, want %d", strA.firstFramenum,
			expectf1)
	}
	expecttA := tA.Add(time.Duration(gap) * ftime)
	if strA.firstTime != expecttA {
		t.Errorf("DataStream.AppendSegment firstTime = %v, want %v", strA.firstTime, expecttA)
	}
}
