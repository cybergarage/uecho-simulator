// Package display renders detached state. It does not schedule refreshes or
// choose a display driver. Future e-paper adapters own batching and intervals.
package display

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/cybergarage/uecho-simulator/internal/model"
)

// RoomSVG is a static 800x480, strictly black/white scene without animation.
func RoomSVG(s model.Snapshot) []byte {
	var b bytes.Buffer
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="800" height="480" viewBox="0 0 800 480"><rect width="800" height="480" fill="white"/><g fill="none" stroke="black" stroke-width="2"><rect x="22" y="72" width="470" height="346" rx="8"/><path d="M45 260h178v104H45zM68 260v-28h132v28M74 288h120M330 304h106v65H330z"/><path d="M260 73v66"/></g>`)
	text := func(x, y int, size int, t string) {
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="%d" font-family="monospace" fill="black">%s</text>`, x, y, size, template.HTMLEscapeString(t))
	}
	text(22, 34, 22, "UECHO / LIVING ROOM")
	text(22, 55, 12, fmt.Sprintf("STATIC 800x480 MONOCHROME / revision %d", s.Revision))
	for _, d := range s.Devices {
		power := "OFF"
		if d.Power {
			power = "ON"
		}
		switch d.Kind {
		case "light":
			fill := "white"
			if d.Power {
				fill = "black"
			}
			fmt.Fprintf(&b, `<circle cx="260" cy="160" r="22" fill="%s" stroke="black" stroke-width="2"/>`, fill)
			text(198, 209, 14, fmt.Sprintf("LIGHT %s %d%%", power, d.Level))
		case "aircon":
			b.WriteString(`<rect x="330" y="92" width="130" height="42" rx="6" fill="white" stroke="black" stroke-width="2"/><path d="M345 122h100" stroke="black"/>`)
			text(331, 157, 14, "AC "+power)
			text(331, 178, 13, fmt.Sprintf("%s / %s", d.Mode, d.TargetLabel()))
		}
	}
	b.WriteString(`<rect x="45" y="92" width="88" height="80" rx="5" fill="white" stroke="black" stroke-width="2"/>`)
	text(52, 114, 11, "ROOM TEMP")
	text(52, 144, 18, fmt.Sprintf("%.1f C", float64(s.AmbientTenths)/10))
	text(520, 104, 17, "DEVICE STATE")
	y := 138
	for _, d := range s.Devices {
		text(520, y, 13, d.Name)
		text(520, y+20, 11, fmt.Sprintf("EOJ %06X / %d EPCs", d.EOJ, len(d.Properties)))
		switch d.Kind {
		case "light":
			text(520, y+42, 14, fmt.Sprintf("on=%t / %d%%", d.Power, d.Level))
		case "aircon":
			text(520, y+42, 14, fmt.Sprintf("on=%t %s %s", d.Power, d.Mode, d.TargetLabel()))
		case "sensor":
			text(520, y+42, 14, fmt.Sprintf("%.1f C / GET only", float64(s.AmbientTenths)/10))
		}
		y += 86
	}
	text(22, 450, 12, "Prototype subset / no auto-refresh / no GPIO or display driver")
	b.WriteString(`</svg>`)
	return b.Bytes()
}
