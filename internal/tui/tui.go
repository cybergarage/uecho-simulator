// Package tui provides selection and plain interfaces for offline virtual devices.
package tui

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/wire"
)

type UI struct {
	Store  *model.Store
	Engine *wire.Engine
	Plain  bool
}

func (u *UI) Draw(out io.Writer) {
	s := u.Store.Snapshot()
	if !u.Plain {
		fmt.Fprint(out, "\x1b[2J\x1b[H")
	}
	fmt.Fprintf(out, "UECHO SIMULATOR | %s | revision %d\nRoom temperature: %.1f C\n", s.Room, s.Revision, float64(s.AmbientTenths)/10)
	for _, d := range s.Devices {
		switch d.Kind {
		case "light":
			fmt.Fprintf(out, "LIGHT  on=%t brightness=%d%%\n", d.Power, d.Level)
		case "aircon":
			fmt.Fprintf(out, "AIRCON on=%t mode=%s target=%d C\n", d.Power, d.Mode, d.Target)
		case "sensor":
			fmt.Fprintf(out, "SENSOR %.1f C (read only)\n", float64(s.AmbientTenths)/10)
		}
	}
	fmt.Fprintln(out, "Commands: light on|off|0..100; ac on|off|cool|heat|fan|16..30; temp -20..50; events; demo; help; quit")
}
func (u *UI) Command(line string, out io.Writer) (bool, error) {
	p := strings.Fields(line)
	if len(p) == 0 {
		return false, nil
	}
	if len(p) == 1 {
		switch p[0] {
		case "quit":
			return true, nil
		case "help":
			u.Draw(out)
			return false, nil
		case "demo":
			return false, u.Demo()
		case "events":
			for _, e := range u.Store.Snapshot().Events {
				fmt.Fprintf(out, "%03d %s %-5s %-8s %s", e.Sequence, e.Time, e.Kind, e.Source, e.Message)
				if e.Frame != "" {
					fmt.Fprintf(out, " [%s]", e.Frame)
				}
				fmt.Fprintln(out)
			}
			return false, nil
		}
	}
	if len(p) != 2 {
		return false, fmt.Errorf("use help for commands")
	}
	switch p[0] {
	case "light", "ac":
		eoj := model.Light
		epc := byte(0xB0)
		if p[0] == "ac" {
			eoj = model.Aircon
			epc = 0xB3
		}
		var value byte
		switch p[1] {
		case "on":
			epc = 0x80
			value = 0x30
		case "off":
			epc = 0x80
			value = 0x31
		case "cool", "heat", "fan":
			if eoj != model.Aircon {
				return false, fmt.Errorf("light has no mode")
			}
			epc = 0xB0
			value = map[string]byte{"cool": 0x42, "heat": 0x43, "fan": 0x45}[p[1]]
		default:
			n, err := strconv.Atoi(p[1])
			if err != nil || n < 0 || n > 255 {
				return false, fmt.Errorf("invalid value")
			}
			value = byte(n)
		}
		_, err := u.Engine.Request(eoj, epc, []byte{value}, true, "tui")
		return false, err
	case "temp":
		v, err := strconv.ParseFloat(p[1], 64)
		if err != nil || !(v >= -20 && v <= 50) || math.Abs(v*10-math.Round(v*10)) > 1e-9 {
			return false, fmt.Errorf("temperature requires -20..50 in tenths")
		}
		return false, u.Store.InjectAmbient(int(math.Round(v*10)), "tui")
	default:
		return false, fmt.Errorf("unknown command %q", p[0])
	}
}

// Demo runs deterministic local inputs through in-memory protocol frames.
func (u *UI) Demo() error {
	for _, op := range []struct {
		eoj        uint32
		epc, value byte
	}{{model.Light, 0x80, 0x30}, {model.Light, 0xB0, 75}, {model.Aircon, 0x80, 0x30}, {model.Aircon, 0xB0, 0x42}, {model.Aircon, 0xB3, 24}} {
		if _, err := u.Engine.Request(op.eoj, op.epc, []byte{op.value}, true, "evening"); err != nil {
			return err
		}
	}
	return u.Store.InjectAmbient(265, "evening")
}
