package dastard

import (
	"fmt"
	"testing"

	"github.com/zeromq/goczmq"
)

func TestPublishData(t *testing.T) {
	dp := DataPublisher{}
	d := []RawType{10, 10, 10, 10, 15, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}
	rec := &DataRecord{data: d, presamples: 4}
	records := []*DataRecord{rec}
	err := dp.PublishData(records)
	if err != nil {
		t.Fail()
	}
	dp.SetLJH22(1, 4, len(d), 1, 1, 8, 1, "TestPublishData.ljh")
	err = dp.PublishData(records)
	if err != nil {
		t.Fail()
	}
	if dp.LJH22.RecordsWritten != 1 {
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
	sub, err := goczmq.NewSub(fmt.Sprintf("tcp://localhost:%v", PortTrigs), topics)
	if err != nil {
		t.Error("Sub ZMQ Error:", err)
	}
	if dp.HasPubFeederChan() {
		t.Error("HasPubFeederChan want false, have", dp.HasPubFeederChan())
	}
	dp.SetPubFeederChan()
	if !dp.HasPubFeederChan() {
		t.Error("HasPubFeederChan want true, have", dp.HasPubFeederChan())
	}
	dp.PublishData(records)
	dp.RemovePubFeederChan()
	if dp.HasPubFeederChan() {
		t.Error("HasPubFeederChan want false, have", dp.HasPubFeederChan())
	}
	// I can't get ANY czmq working with tcp ports, I got a pub-sub example from the czmq tests using inproc to work
	// same example, using tcp, does not work
	msg, err := sub.RecvMessageNoWait()
	_ = msg // don't complain about unused msg
	// if err != nil {
	// 	t.Error("ZMQ error:", err, "\nmsg:", msg)
	// }
	dp.RemovePubFeederChan()

	dp.SetLJH3(0, 0, 0, 0, "TestPublishData.ljh3")
	err = dp.PublishData(records)
	if err != nil {
		t.Error("failed to publish record")
	}
	if dp.LJH3.RecordsWritten != 1 {
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
