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
		data := make([]RawType, n)
		segN := NewDataSegment(data, 1, 0, time.Now(), time.Millisecond)
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
		data := make([]RawType, n)
		strN := NewDataStream(data, 1, 0, time.Now(), time.Microsecond)
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
	strA := NewDataStream(dA, 1, 0, tA, ftime)
	strB := NewDataSegment(dB, 1, FrameIndex(len(dA)), tB, ftime)

	strA.AppendSegment(strB)
	if len(strA.rawData) != len(dC) {
		t.Errorf("DataStream.AppendSegment result was length %d, want %d", len(strA.rawData), len(dC))
	}
	if strA.samplesSeen != len(dC) {
		t.Errorf("DataStream.AppendSegment samplesSeen was length %d, want %d", strA.samplesSeen, len(dC))
	}
	if !reflect.DeepEqual(strA.rawData, dC) {
		t.Errorf("DataStream.AppendSegment result was %v, want %v", strA.rawData, dC)
	}
	expectf1 := strB.firstFrameIndex - FrameIndex(len(dA))
	if strA.firstFrameIndex != expectf1 {
		t.Errorf("DataStream.AppendSegment firstFrameIndex = %d, want %d", strA.firstFrameIndex,
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
		if strA.samplesSeen != len(dC) {
			t.Errorf("DataStream.TrimKeepingN samplesSeen was length %d, want %d", strA.samplesSeen, len(dC))
		}
		if !reflect.DeepEqual(strA.rawData, expectedData) {
			t.Errorf("DataStream.TrimKeepingN result was %v, want %v", strA.rawData, expectedData)
		}
		if cap(strA.rawData) < len(dC) {
			t.Errorf("DataStream.TrimKeepingN left cap(rawData)=%d, want at least %d", cap(strA.rawData), len(dC))
		}
		expectf1 := FrameIndex(len(dC) - len(strA.rawData))
		if strA.firstFrameIndex != expectf1 {
			t.Errorf("DataStream.TrimKeepingN firstFrameIndex = %d, want %d", strA.firstFrameIndex,
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
	fA := FrameIndex(100)
	fB := FrameIndex(len(dA)+gap) + fA
	tA := time.Now()
	tB := tA.Add(ftime * time.Duration(len(dA)+gap))
	strA := NewDataStream(dA, 1, fA, tA, ftime)
	strB := NewDataSegment(dB, 1, fB, tB, ftime)

	strA.AppendSegment(strB)
	if len(strA.rawData) != len(dC) {
		t.Errorf("DataStream.AppendSegment result was length %d, want %d", len(strA.rawData), len(dC))
	}
	if !reflect.DeepEqual(strA.rawData, dC) {
		t.Errorf("DataStream.AppendSegment result was %v, want %v", strA.rawData, dC)
	}
	expectf1 := fA + FrameIndex(gap)
	if strA.firstFrameIndex != expectf1 {
		t.Errorf("DataStream.AppendSegment firstFrameIndex = %d, want %d", strA.firstFrameIndex,
			expectf1)
	}
	expecttA := tA.Add(time.Duration(gap) * ftime)
	if strA.TimeOf(0) != expecttA {
		t.Errorf("DataStream.AppendSegment firstTime = %v, want %v", strA.firstTime, expecttA)
	}
}

func TestStreamDecimated(t *testing.T) {
	ftime := time.Second
	dA := []RawType{6, 4, 2, 5, 1, 0}
	dB := []RawType{10, 7, 8, 9, 10}
	// dC := append(dA, dB...)
	fA := FrameIndex(100)
	tA := time.Now()

	decimations := []int{1, 2, 3, 5}
	for _, decimationA := range decimations {
		aframes := len(dA) * decimationA
		for _, decimationB := range decimations {
			strA := NewDataStream(dA, decimationA, fA, tA, ftime)
			fB := fA + FrameIndex(aframes)
			tB := tA.Add(ftime * time.Duration(aframes))
			strB := NewDataSegment(dB, decimationB, fB, tB, ftime)

			strA.AppendSegment(strB)
			expectf1 := fB - FrameIndex(len(dA)*decimationB)
			if strA.firstFrameIndex != expectf1 {
				t.Errorf("DataStream.AppendSegment firstFrameIndex = %d, want %d with dec %d, %d",
					strA.firstFrameIndex, expectf1, decimationA, decimationB)
			}
			expecttA := tA.Add(ftime * time.Duration(len(dA)*(decimationA-decimationB)))
			if strA.TimeOf(0) != expecttA {
				t.Errorf("DataStream.AppendSegment firstTime = %v, want %v %d, %d",
					strA.firstTime, expecttA, decimationA, decimationB)
			}
		}

	}
}

func TestDecimation(t *testing.T) {
	N := 100
	data := make([]RawType, N)
	for i := 0; i < N; i++ {
		data[i] = RawType(i * 2)
	}

	for _, useAvg := range []bool{true, false} {
		for _, decimation := range []int{1, 2, 3, 4, 6} {
			ch := new(DataStreamProcessor)
			ch.DecimateLevel = decimation
			ch.Decimate = decimation > 1
			ch.DecimateAvgMode = useAvg
			dcopy := make([]RawType, N)
			copy(dcopy, data)
			seg := &DataSegment{rawData: dcopy, framesPerSample: 1, signed: false}
			ch.DecimateData(seg)
			if seg.framesPerSample != decimation {
				t.Errorf("DataChannel.DecimateData did not alter framesPerSample = %d, want %d",
					seg.framesPerSample, decimation)
			}
			expect := (len(data) + decimation - 1) / decimation
			if len(seg.rawData) != expect {
				t.Errorf("DataChannel.DecimateData data length = %d, want %d",
					len(seg.rawData), expect)
			}
			for i := 0; i < N/decimation; i++ {
				expect := i * 2 * decimation
				if useAvg {
					expect += decimation - 1
				}
				if seg.rawData[i] != RawType(expect) {
					t.Errorf("DataChannel.DecimateData (avg=%v, dec=%d) data[%d] = %d, want %d",
						useAvg, decimation, i, seg.rawData[i], expect)
				}
			}
		}
	}
}
