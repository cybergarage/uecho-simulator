package model

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMandatoryProfilesAndMaps(t *testing.T) {
	s := New()
	common := []byte{0x80, 0x81, 0x82, 0x88, 0x8a, 0x9d, 0x9e, 0x9f}
	for _, eoj := range []uint32{Light, Aircon, Sensor, Node} {
		codes := common
		if eoj == Node {
			codes = []byte{0x80, 0x82, 0x83, 0x8a, 0x9d, 0x9e, 0x9f, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7}
		}
		if eoj == Light {
			codes = append(append([]byte{}, codes...), 0xb6)
		}
		if eoj == Aircon {
			codes = append(append([]byte{}, codes...), 0x8f, 0xa0, 0xb0, 0xb3)
		}
		if eoj == Sensor {
			codes = append(append([]byte{}, codes...), 0xe0)
		}
		get, _ := s.Read(eoj, 0x9f)
		for _, epc := range codes {
			b, err := s.Read(eoj, epc)
			if err != nil || len(b) == 0 || !bytes.Contains(get[1:], []byte{epc}) {
				t.Fatalf("%06x/%02x missing: %x %v", eoj, epc, b, err)
			}
		}
		for _, mapEPC := range []byte{0x9d, 0x9e, 0x9f} {
			data, _ := s.Read(eoj, mapEPC)
			if len(data) != int(data[0])+1 {
				t.Fatalf("map %x", data)
			}
			for _, epc := range data[1:] {
				_, def, err := s.definition(eoj, epc)
				if err != nil || mapEPC == 0x9e && !def.Writable || mapEPC == 0x9d && !def.Announce {
					t.Fatalf("false map declaration %06x/%02x", eoj, epc)
				}
			}
		}
	}
	for epc, want := range map[byte][]byte{0xd3: {0, 0, 3}, 0xd4: {0, 4}, 0xd5: {3, 2, 0x90, 1, 1, 0x30, 1, 0, 0x11, 1}, 0xd6: {3, 2, 0x90, 1, 1, 0x30, 1, 0, 0x11, 1}, 0xd7: {3, 2, 0x90, 1, 0x30, 0, 0x11}, 0x82: {1, 14, 1, 0}} {
		got, _ := s.Read(Node, epc)
		if !bytes.Equal(got, want) {
			t.Fatalf("node/%02x %x want %x", epc, got, want)
		}
	}
	for _, eoj := range []uint32{Light, Aircon, Sensor} {
		b, _ := s.Read(eoj, 0x82)
		if !bytes.Equal(b, []byte{0, 0, 'R', 4}) {
			t.Fatalf("version %x", b)
		}
	}
	if err := s.Write(Sensor, 0x80, []byte{0x31}, "test"); err == nil {
		t.Fatal("sensor status writable")
	}
}
func TestMapBoundaryAndUpperEPCRoundTrip(t *testing.T) {
	for _, count := range []int{15, 16, 17, 128} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			codes := []byte{}
			// Include FF/C0 at the 15/16 boundary, then cover all 128 EPCs.
			for _, epc := range []byte{0xff, 0xc0, 0xe0, 0xd6} {
				codes = append(codes, epc)
			}
			for n := 0x80; len(codes) < count; n++ {
				if !bytes.Contains(codes, []byte{byte(n)}) {
					codes = append(codes, byte(n))
				}
			}
			encoded := propertyMap(codes)
			if count < 16 && len(encoded) != count+1 || count >= 16 && len(encoded) != 17 {
				t.Fatalf("format %x", encoded)
			}
			decoded := append([]byte(nil), encoded[1:]...)
			if count >= 16 {
				decoded = nil
				for low, mask := range encoded[1:] {
					for bit := 0; bit < 8; bit++ {
						if mask&(1<<bit) != 0 {
							decoded = append(decoded, byte(0x80+16*bit+low))
						}
					}
				}
				if encoded[16]&0x80 == 0 || encoded[1]&0x50 != 0x50 {
					t.Fatalf("upper EPC bits missing: %x", encoded)
				}
			}
			if len(decoded) != count {
				t.Fatalf("decode %x", decoded)
			}
			for _, epc := range codes {
				found := false
				for _, v := range decoded {
					if byte(v) == epc {
						found = true
					}
				}
				if !found {
					t.Fatalf("lost upper EPC %02x in %x", epc, encoded)
				}
			}
		})
	}
}
func TestProfileValuesAndOrderedAnnouncements(t *testing.T) {
	s := New()
	changes, cancel := s.SubscribeNotifications()
	defer cancel()
	for _, v := range []byte{0, 50} {
		if err := s.Write(Aircon, 0xb3, []byte{v}, "test"); err != nil {
			t.Fatal(err)
		}
		c := <-changes
		if c.EOJ != Aircon || c.EPC != 0xb3 || c.Data[0] != v {
			t.Fatal(c)
		}
	}
	for _, v := range []byte{51, 0xfd, 0xfe, 0xff} {
		if s.Write(Aircon, 0xb3, []byte{v}, "test") == nil {
			t.Fatalf("accepted %x", v)
		}
	}
	for _, v := range []byte{0x40, 0x41, 0x42, 0x43, 0x44, 0x45} {
		if s.Write(Aircon, 0xb0, []byte{v}, "test") != nil {
			t.Fatal(v)
		}
		<-changes
	}
	if s.Write(Light, 0xb6, []byte{0x41}, "test") == nil {
		t.Fatal("automatic lighting unsupported")
	}
	if s.Write(Light, 0xb6, []byte{0x42}, "test") != nil {
		t.Fatal("main lighting")
	}
	loc := make([]byte, 17)
	loc[0] = 1
	if s.Write(Light, 0x81, loc, "test") != nil {
		t.Fatal("position location")
	}
	<-changes
	if s.Write(Light, 0x81, []byte{2}, "test") == nil {
		t.Fatal("reserved location")
	}
	if err := s.InjectAmbient(-125, "test"); err != nil {
		t.Fatal(err)
	}
	for _, eoj := range []uint32{Sensor, Aircon} {
		c := <-changes
		if c.EOJ != eoj {
			t.Fatal(c)
		}
	}
	if err := s.InjectAmbient(-125, "test"); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-changes:
		t.Fatalf("unchanged value announced %v", c)
	default:
	}
}
func TestSlowAnnouncementConsumerFails(t *testing.T) {
	s := New()
	changes, cancel := s.SubscribeNotifications()
	defer cancel()
	for n := 0; n < 258; n++ {
		if err := s.Write(Light, 0xb0, []byte{byte(n % 101)}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for range changes {
		count++
	}
	if count != 256 {
		t.Fatalf("queue %d", count)
	}
}

func TestStableDistinctProcessIdentification(t *testing.T) {
	s := New()
	seen := map[string]bool{}
	for _, eoj := range []uint32{Node, Light, Aircon, Sensor} {
		first, _ := s.Read(eoj, 0x83)
		second, _ := s.Read(eoj, 0x83)
		if len(first) != 17 || !bytes.Equal(first[:4], []byte{0xfe, 0xff, 0xff, 0xff}) || !bytes.Equal(first, second) || seen[string(first)] {
			t.Fatalf("identification %06x %x", eoj, first)
		}
		seen[string(first)] = true
	}
}
