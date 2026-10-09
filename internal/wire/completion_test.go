package wire

import (
	"github.com/cybergarage/uecho-simulator/internal/model"
	"testing"
)

func TestDeviceCompletionRegression(t *testing.T) {
	for _, c := range []struct {
		name, request string
		esv           byte
	}{
		{"node discovery", "1081000105ff010ef0006201d600", 0x72},
		{"installation location", "1081000105ff0102900162018100", 0x72},
		{"lighting mode", "1081000105ff010290016201b600", 0x72},
		{"power saving", "1081000105ff0101300162018f00", 0x72},
		{"air flow", "1081000105ff010130016201a000", 0x72},
		{"notification request", "1081000105ff0102900163018000", 0x73},
		{"SetGet", "1081000105ff010290016e01800130018000", 0x7e},
		{"INFC acknowledgement", "1081000105ff010ef0017401800130", 0x7a},
	} {
		t.Run(c.name, func(t *testing.T) {
			reply, err := New(model.New()).Handle(frame(t, c.request), "controller")
			if err != nil || len(reply) < 12 || reply[10] != c.esv {
				t.Fatalf("reply %x error %v; need ESV %02x", reply, err, c.esv)
			}
		})
	}
	t.Run("SetI changes state without response", func(t *testing.T) {
		s := model.New()
		e := New(s)
		reply, err := e.Handle(frame(t, "1081000105ff010290016001800130"), "controller")
		value, _ := s.Read(model.Light, 0x80)
		if err != nil || reply != nil || value[0] != 0x30 {
			t.Fatalf("reply %x state %x err %v", reply, value, err)
		}
	})
}
