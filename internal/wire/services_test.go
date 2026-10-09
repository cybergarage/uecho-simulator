package wire

import (
	"bytes"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"testing"
)

func TestServiceErrorsAndPartialSuccess(t *testing.T) {
	for _, c := range []struct{ req, want string }{
		{"1081000105ff010290016002800130b00165", "1081000102900105ff0150028000b00165"},
		{"1081000105ff0102900162028000ee00", "1081000102900105ff015202800131ee00"},
		{"1081000105ff0102900163028000ee00", "1081000102900105ff015302800131ee00"},
		{"1081000105ff010290016e01800130028000ee00", "1081000102900105ff015e01800002800130ee00"},
		{"1081000105ff010290016e01b0016501b000", "1081000102900105ff015e01b0016501b0013c"},
		{"1081000105ff010290016e02800130b0014b028000b000", "1081000102900105ff017e028000b00002800130b0014b"},
		{"1081000105ff010ef0017402ee0130800131", "108100010ef00105ff017a02ee008000"},
	} {
		s := model.New()
		got, err := New(s).Handle(frame(t, c.req), "fixture")
		if err != nil || !bytes.Equal(got, frame(t, c.want)) {
			t.Fatalf("request %s reply %x want %s err %v", c.req, got, c.want, err)
		}
		if got[10] == 0x7a {
			b, _ := s.Read(model.Node, 0x80)
			if b[0] != 0x30 {
				t.Fatal("INFC mutated receiver")
			}
		}
	}
}
func TestWildcardAndIgnoredResponses(t *testing.T) {
	e := New(model.New())
	for _, req := range []string{"1081000105ff0102900062018000", "1081000105ff010ef0006201d600"} {
		got, err := e.HandleAll(frame(t, req), "test")
		if err != nil || len(got) != 1 || got[0][6] != 1 || got[0][10] != 0x72 {
			t.Fatalf("wildcard %x %v", got, err)
		}
	}
	before := e.Store().Snapshot()
	for _, req := range []string{"108100010290010ef0017301800130", "1081000105ff010ef0017201800130", "1081000105ff010ef0015e0000"} {
		got, err := e.Handle(frame(t, req), "test")
		if err != nil || got != nil {
			t.Fatalf("response %x %v", got, err)
		}
	}
	if len(e.Store().Snapshot().Events) != len(before.Events) {
		t.Fatal("responses counted as requests")
	}
}
func TestSetGetStructureAndBoundedResponses(t *testing.T) {
	e := New(model.New())
	valid := frame(t, "1081000105ff010290016e01800130018000")
	for n := 0; n < len(valid); n++ {
		if _, err := e.Handle(valid[:n], "bad"); err == nil {
			t.Fatalf("truncation %d", n)
		}
	}
	for _, b := range [][]byte{append(bytes.Clone(valid), 0), frame(t, "1081000105ff010290016e0000"), frame(t, "1081000105ff010290016e0180013000")} {
		if _, err := e.Handle(b, "bad"); err == nil {
			t.Fatalf("accepted %x", b)
		}
	}
	// 100 repeated 17-byte IDs would exceed the 1024-byte frame limit.
	req := frame(t, "1081000105ff010290016264")
	for i := 0; i < 100; i++ {
		req = append(req, 0x83, 0)
	}
	got, err := e.Handle(req, "test")
	if err != nil || len(got) > 1024 || got[10] != 0x52 || got[11] != 53 {
		t.Fatalf("bounded response len=%d %x %v", len(got), got[:12], err)
	}
	if err := validateFrame(got); err != nil {
		t.Fatal(err)
	}
	req = frame(t, "1081000105ff010290016e0180013064")
	for i := 0; i < 100; i++ {
		req = append(req, 0x83, 0)
	}
	got, err = e.Handle(req, "test")
	if err != nil || len(got) > 1024 || got[10] != 0x5e {
		t.Fatalf("bounded SetGet %d %v", len(got), err)
	}
	if err := validateFrame(got); err != nil {
		t.Fatal(err)
	}
}
