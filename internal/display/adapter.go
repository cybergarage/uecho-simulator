package display

import "github.com/cybergarage/uecho-simulator/internal/model"

// Frame carries output without choosing hardware or refresh policy.
type Frame struct {
	Width, Height int
	MediaType     string
	Data          []byte
}

// Adapter consumes detached state; hardware adapters own conversion and refresh.
type Adapter interface {
	Render(model.Snapshot) (Frame, error)
}
type SVG struct{}

func (SVG) Render(s model.Snapshot) (Frame, error) {
	return Frame{800, 480, "image/svg+xml", RoomSVG(s)}, nil
}
