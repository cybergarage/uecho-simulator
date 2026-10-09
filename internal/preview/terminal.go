package preview

import (
	"context"
	"fmt"
	"os"

	"github.com/cybergarage/uecho-simulator/internal/display"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/gdamore/tcell/v2"
)

// Terminal keys affect only this display process, never device properties.
func Terminal(ctx context.Context, screen tcell.Screen, store *model.Store, url, export string) error {
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	message := "q / Ctrl-C quit • ? help • r redraw • s save SVG"
	draw := func() {
		screen.Clear()
		lines := []string{"UECHO / READ-ONLY ROOM DISPLAY", url, fmt.Sprintf("Model revision %d", store.Snapshot().Revision), message}
		for y, line := range lines {
			for x, r := range []rune(line) {
				screen.SetContent(x, y, r, nil, tcell.StyleDefault.Bold(true))
			}
		}
		screen.Show()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = screen.PostEvent(tcell.NewEventInterrupt(nil))
		case <-done:
		}
	}()
	draw()
	for {
		event := screen.PollEvent()
		if event == nil {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
		switch e := event.(type) {
		case *tcell.EventResize:
			screen.Sync()
			draw()
		case *tcell.EventKey:
			if e.Key() == tcell.KeyCtrlC || e.Rune() == 'q' {
				return nil
			}
			switch e.Rune() {
			case '?':
				message = "q / Ctrl-C: stop • r: redraw current state • s: save room.svg"
				draw()
			case 'r':
				store.Log("DISPLAY", "preview", "redraw requested", nil)
				message = "Current model redrawn"
				draw()
			case 's':
				frame, err := (display.SVG{}).Render(store.Snapshot())
				if err == nil {
					err = os.WriteFile(export, frame.Data, 0644)
				}
				if err != nil {
					message = "Save failed: " + err.Error()
				} else {
					message = "Saved " + export
				}
				draw()
			}
		}
	}
}
