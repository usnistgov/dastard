package dastard

import (
	"testing"
	"time"

	czmq "github.com/zeromq/goczmq"
)

func TestPublishData(t *testing.T) {
	// This block is copied from the goczmq TestPublishData
	// without it, this test fails with a timeout when trying to recieve a message
	// from pub
	// I'm guessing there is some sort of initilization in the zmq library that does?
	func() {
		pubz := czmq.NewPubChanneler("inproc://channelerpubsubz")
		defer pubz.Destroy()

		subz := czmq.NewSubChanneler("inproc://channelerpubsubz", "a,b")
		defer subz.Destroy()

		pubz.SendChan <- [][]byte{[]byte("a"), []byte("message")}
		select {
		case resp := <-subz.RecvChan:
			topic, message := string(resp[0]), string(resp[1])
			if want, got := "a", topic; want != got {
				t.Errorf("want '%s', got '%s'", want, got)
			}
			if want, got := "message", message; want != got {
				t.Errorf("want '%s', got '%s'", want, got)
			}
		case <-time.After(time.Second * 2):
			t.Errorf("timeout")
		}
	}()

	dp := DataPublisher{}
	d := []RawType{10, 10, 10, 10, 15, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}
	rec := &DataRecord{data: d, presamples: 4}
	records := []*DataRecord{rec, rec, rec}
	err := dp.PublishData(records)
	if err != nil {
		t.Fail()
	}
	dp.SetLJH22(1, 4, len(d), 1, 1, 8, 1, "TestPublishData.ljh")
	err = dp.PublishData(records)
	if err != nil {
		t.Fail()
	}
	if dp.LJH22.RecordsWritten != 3 {
		t.Fail()
	}
	if !dp.HasLJH22() {
		t.Error("HasLJH22 want true, have", dp.HasLJH22())
	}
	dp.RemoveLJH22()
	if dp.HasLJH22() {
		t.Error("HasLJH22 want false, have", dp.HasLJH22())
	}
	// ZMQ publishing
	topics := "" // comma delimted list of topics to subscribe to, empty strings subscribes to all topics
	inprocEndpoint := "inproc://channelerpubsubRecords"
	sub := czmq.NewSubChanneler(inprocEndpoint, topics)
	defer sub.Destroy()
	if dp.HasPubRecords() {
		t.Error("HasPubRecords want false, have", dp.HasPubRecords())
	}
	dp.SetPubRecordsWithHostname(inprocEndpoint)

	if !dp.HasPubRecords() {
		t.Error("HasPubRecords want true, have", dp.HasPubRecords())
	}

	dp.PublishData(records)

	dp.RemovePubRecords()
	if dp.HasPubRecords() {
		t.Error("HasPubRecords want false, have", dp.HasPubRecords())
	}
	for i := 0; i < 3; i++ {
		select {
		case msg := <-sub.RecvChan:
			if len(msg) != 2 {
				t.Error("bad message length")
			}
		case <-time.After(time.Second * 1):
			t.Errorf("timeout")
		}
	}

	inprocEndpoint = "inproc://channelerpubsubSummaries"
	sub = czmq.NewSubChanneler(inprocEndpoint, topics)
	defer sub.Destroy()
	if dp.HasPubSummaries() {
		t.Error("HasPubSummaries want false, have", dp.HasPubSummaries())
	}
	dp.SetPubSummariesWithHostname(inprocEndpoint)

	if !dp.HasPubSummaries() {
		t.Error("HasPubSummaries want true, have", dp.HasPubSummaries())
	}

	dp.PublishData(records)

	dp.RemovePubSummaries()
	if dp.HasPubSummaries() {
		t.Error("HasPubSummaries want false, have", dp.HasPubSummaries())
	}
	for i := 0; i < 3; i++ {
		select {
		case msg := <-sub.RecvChan:
			if len(msg) != 2 {
				t.Error("bad message length")
			}
		case <-time.After(time.Second * 1):
			t.Errorf("timeout")
		}
	}

	dp.SetLJH3(0, 0, 0, 0, "TestPublishData.ljh3")
	err = dp.PublishData(records)
	if err != nil {
		t.Error("failed to publish record")
	}
	if dp.LJH3.RecordsWritten != 3 {
		t.Error("wrong number of RecordsWritten, want 1, have", dp.LJH3.RecordsWritten)
	}
	if !dp.HasLJH3() {
		t.Error("HasLJH3 want true, have", dp.HasLJH3())
	}
	dp.RemoveLJH3()
	if dp.HasLJH3() {
		t.Error("HasLJH3 want false, have", dp.HasLJH3())
	}
}
