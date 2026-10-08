package display

import (
	"bytes"
	"encoding/xml"
	"io"
	"testing"

	"github.com/cybergarage/uecho-simulator/internal/model"
)

func TestMonochromeFrame(t *testing.T) {
	s := model.New().Snapshot()
	s.Room = "unused"
	s.Devices[0].Name = "<script>&"
	f, err := (SVG{}).Render(s)
	if err != nil || f.Width != 800 || f.Height != 480 || f.MediaType != "image/svg+xml" {
		t.Fatal("invalid frame")
	}
	decoder := xml.NewDecoder(bytes.NewReader(f.Data))
	root := false
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if el, ok := tok.(xml.StartElement); ok {
			if el.Name.Local == "svg" {
				root = true
			}
			if el.Name.Local == "script" {
				t.Fatal("unescaped text")
			}
			for _, a := range el.Attr {
				if (a.Name.Local == "fill" || a.Name.Local == "stroke") && a.Value != "black" && a.Value != "white" && a.Value != "none" {
					t.Fatalf("nonmonochrome %s", a.Value)
				}
			}
		}
	}
	if !root || !bytes.Contains(f.Data, []byte("&lt;script&gt;&amp;")) {
		t.Fatal("invalid SVG or escaping")
	}
}
