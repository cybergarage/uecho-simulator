package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/wire"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func dashboard(t *testing.T) (*Dashboard, tcell.SimulationScreen) {
	t.Helper()
	s := model.New()
	d := NewDashboard(s, wire.New(s), nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	d.app.SetScreen(screen)
	screen.SetSize(120, 36)
	d.app.ForceDraw()
	t.Cleanup(screen.Fini)
	return d, screen
}
func press(d *Dashboard, key tcell.Key, r rune) {
	event := d.capture(tcell.NewEventKey(key, r, 0))
	if event != nil {
		if handler := d.pages.InputHandler(); handler != nil {
			handler(event, func(p tview.Primitive) { d.focus(p) })
		}
	}
	d.app.ForceDraw()
}
func text(d *Dashboard, s string) {
	for _, r := range s {
		press(d, tcell.KeyRune, r)
	}
}
func screenText(screen tcell.SimulationScreen) string {
	cells, w, h := screen.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := cells[y*w+x].Runes
			if len(r) == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(r[0])
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Optional local evidence captures real widget cells, not a hand-drawn mockup.
func capture(t *testing.T, screen tcell.SimulationScreen, name string) {
	t.Helper()
	dir := os.Getenv("UECHO_TUI_CAPTURE")
	if dir == "" {
		return
	}
	cells, w, h := screen.GetContents()
	type cell struct {
		Text   string
		FG, BG string
	}
	out := struct {
		Width, Height int
		Cells         []cell
	}{Width: w, Height: h}
	for _, c := range cells {
		fg, bg, _ := c.Style.Decompose()
		out.Cells = append(out.Cells, cell{string(c.Runes), fg.String(), bg.String()})
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
func TestDashboardNavigationFormsAndCancel(t *testing.T) {
	d, screen := dashboard(t)
	before := d.store.Snapshot().Revision
	if !strings.Contains(screenText(screen), "OFFLINE") || !strings.Contains(screenText(screen), "Actions") {
		t.Fatal("missing dashboard")
	}
	capture(t, screen, "dashboard")
	press(d, tcell.KeyEnter, 0)
	if d.app.GetFocus() != d.actions {
		t.Fatal("Enter did not focus actions")
	}
	press(d, tcell.KeyEnter, 0)
	form, ok := d.modal.(*tview.Form)
	if !ok {
		t.Fatal("edit form not opened")
	}
	press(d, tcell.KeyRune, ' ')
	press(d, tcell.KeyTab, 0)
	form.GetFormItem(1).(*tview.InputField).SetText("75")
	d.app.ForceDraw()
	capture(t, screen, "light-form")
	press(d, tcell.KeyEscape, 0)
	if d.modal != nil || d.store.Snapshot().Revision != before || d.app.GetFocus() != d.actions {
		t.Fatal("cancel changed state or focus")
	}
	press(d, tcell.KeyEnter, 0)
	form = d.modal.(*tview.Form)
	press(d, tcell.KeyRune, ' ')
	form.GetFormItem(1).(*tview.InputField).SetText("101")
	form.GetButton(0).InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), d.focus)
	if d.modal == nil || d.store.Snapshot().Revision != before {
		t.Fatal("invalid form was applied")
	}
	form.GetFormItem(1).(*tview.InputField).SetText("75")
	// From checkbox, Tab -> value -> Apply, then Enter.
	press(d, tcell.KeyTab, 0)
	press(d, tcell.KeyTab, 0)
	press(d, tcell.KeyEnter, 0)
	snap := d.store.Snapshot()
	if d.modal != nil || !snap.Devices[0].Power || snap.Devices[0].Level != 75 {
		t.Fatalf("form apply failed %+v", snap.Devices[0])
	}
	for _, e := range snap.Events {
		if e.Kind == "RX" || e.Kind == "TX" {
			t.Fatal("in-memory frames displayed as real traffic")
		}
	}
	capture(t, screen, "light-applied")
}
func TestDashboardSearchSelectionAndSensor(t *testing.T) {
	d, screen := dashboard(t)
	press(d, tcell.KeyDown, 0)
	if d.selected != model.Aircon {
		t.Fatal("arrow selection failed")
	}
	press(d, tcell.KeyRune, '/')
	text(d, "sensor")
	press(d, tcell.KeyEnter, 0)
	if len(d.visible) != 1 || d.selected != model.Sensor || d.app.GetFocus() != d.table {
		t.Fatal("search selected stale device")
	}
	press(d, tcell.KeyEnter, 0)
	press(d, tcell.KeyEnter, 0)
	form := d.modal.(*tview.Form)
	form.GetFormItem(1).(*tview.InputField).SetText("-12.5")
	form.GetButton(0).InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), d.focus)
	d.app.ForceDraw()
	if d.store.Snapshot().AmbientTenths != -125 || d.store.Snapshot().Devices[0].Power {
		t.Fatal("sensor action targeted wrong device")
	}
	if _, err := d.engine.Request(model.Sensor, 0xe0, []byte{1}, true, "test"); err == nil {
		t.Fatal("sensor accepted protocol write")
	}
	press(d, tcell.KeyEscape, 0)
	if len(d.visible) != 3 {
		t.Fatal("Esc did not clear search")
	}
	press(d, tcell.KeyRune, '/')
	text(d, "missing")
	press(d, tcell.KeyEnter, 0)
	if d.selected != 0 || len(d.visible) != 0 {
		t.Fatal("no-match retained a device")
	}
	press(d, tcell.KeyEnter, 0)
	press(d, tcell.KeyEnter, 0)
	if len(d.visible) != 3 {
		t.Fatal("clear-search action failed")
	}
	capture(t, screen, "sensor")
}
func TestDashboardScenarioFocusAndResize(t *testing.T) {
	d, screen := dashboard(t)
	before := d.store.Snapshot().Revision
	press(d, tcell.KeyTab, 0)
	if d.app.GetFocus() != d.actions {
		t.Fatal("Tab")
	}
	press(d, tcell.KeyBacktab, 0)
	if d.app.GetFocus() != d.table {
		t.Fatal("Shift-Tab")
	}
	press(d, tcell.KeyRune, 's')
	if d.modal == nil {
		t.Fatal("scenario not confirmed")
	}
	press(d, tcell.KeyEscape, 0)
	if d.store.Snapshot().Revision != before {
		t.Fatal("scenario cancelled but mutated")
	}
	press(d, tcell.KeyRune, 's')
	capture(t, screen, "scenario-confirm")
	press(d, tcell.KeyTab, 0)
	press(d, tcell.KeyEnter, 0)
	if d.store.Snapshot().AmbientTenths != 265 || !d.store.Snapshot().Devices[1].Power {
		t.Fatal("scenario not applied")
	}
	press(d, tcell.KeyDown, 0)
	selected := d.selected
	for _, size := range [][2]int{{70, 24}, {50, 20}, {30, 10}, {120, 36}} {
		screen.SetSize(size[0], size[1])
		d.app.ForceDraw()
		if d.selected != selected {
			t.Fatal("resize lost selection")
		}
		if size[0] == 50 {
			capture(t, screen, "compact")
		}
	}
	press(d, tcell.KeyEnter, 0)
	press(d, tcell.KeyEnter, 0)
	screen.SetSize(50, 20)
	d.app.ForceDraw()
	capture(t, screen, "compact-form")
	press(d, tcell.KeyEscape, 0)
	press(d, tcell.KeyRune, 'q')
	press(d, tcell.KeyEnter, 0)
	if d.modal != nil {
		t.Fatal("default quit button should cancel")
	}
}
func TestDashboardPreviewCallback(t *testing.T) {
	s := model.New()
	calls := 0
	d := NewDashboard(s, wire.New(s), func(model.Snapshot) error { calls++; return os.ErrPermission })
	screen := tcell.NewSimulationScreen("UTF-8")
	d.app.SetScreen(screen)
	screen.SetSize(100, 30)
	defer screen.Fini()
	d.app.ForceDraw()
	d.scenario() // Modal callbacks are exercised via keys below.
	press(d, tcell.KeyTab, 0)
	press(d, tcell.KeyEnter, 0)
	if calls != 1 || !strings.Contains(d.status.GetText(false), "Preview") {
		t.Fatal("preview failure was not surfaced")
	}
}
func TestDashboardRunCancellationFinalizesScreen(t *testing.T) {
	s := model.New()
	d := NewDashboard(s, wire.New(s), nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	d.app.SetScreen(screen)
	screen.SetSize(100, 30)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	var once sync.Once
	d.app.SetAfterDrawFunc(func(tcell.Screen) { once.Do(func() { close(ready) }) })
	result := make(chan error, 1)
	go func() { result <- d.Run(ctx) }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("dashboard did not start")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dashboard did not restore/stop")
	}
	if screen.PollEvent() != nil {
		t.Fatal("screen was not finalized")
	}
}

func TestDashboardAirconForm(t *testing.T) {
	d, screen := dashboard(t)
	press(d, tcell.KeyDown, 0)
	press(d, tcell.KeyEnter, 0)
	press(d, tcell.KeyEnter, 0)
	form := d.modal.(*tview.Form)
	press(d, tcell.KeyRune, ' ')
	press(d, tcell.KeyTab, 0)
	press(d, tcell.KeyEnter, 0)
	press(d, tcell.KeyDown, 0)
	press(d, tcell.KeyEnter, 0)
	form.GetFormItem(2).(*tview.InputField).SetText("21")
	d.app.ForceDraw()
	capture(t, screen, "aircon-form")
	form.GetButton(0).InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), d.focus)
	d.app.ForceDraw()
	ac := d.store.Snapshot().Devices[1]
	if !ac.Power || ac.Mode != "heat" || ac.Target != 21 {
		t.Fatalf("AC form failed %+v", ac)
	}
	capture(t, screen, "aircon-applied")
}

func TestDashboardHelpAndCompactConfirmation(t *testing.T) {
	d, screen := dashboard(t)
	screen.SetSize(45, 18)
	d.app.ForceDraw()
	press(d, tcell.KeyRune, '?')
	if d.modal == nil {
		t.Fatal("help not opened")
	}
	press(d, tcell.KeyDown, 0)
	press(d, tcell.KeyTab, 0)
	press(d, tcell.KeyEscape, 0)
	if d.modal != nil || d.app.GetFocus() != d.table {
		t.Fatal("help did not restore focus")
	}
	press(d, tcell.KeyRune, 's')
	d.app.ForceDraw()
	capture(t, screen, "minimum-confirm")
	press(d, tcell.KeyEscape, 0)
	if d.store.Snapshot().AmbientTenths != 220 {
		t.Fatal("compact confirmation applied on cancel")
	}
}

func TestDashboardRunQuitConfirmation(t *testing.T) {
	s := model.New()
	d := NewDashboard(s, wire.New(s), nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	d.app.SetScreen(screen)
	screen.SetSize(100, 30)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	var once sync.Once
	d.app.SetAfterDrawFunc(func(tcell.Screen) { once.Do(func() { close(ready) }) })
	result := make(chan error, 1)
	go func() { result <- d.Run(ctx) }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("not started")
	}
	screen.InjectKey(tcell.KeyRune, 'q', 0)
	screen.InjectKey(tcell.KeyTab, 0, 0)
	screen.InjectKey(tcell.KeyEnter, 0, 0)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("quit confirmation did not exit through root event loop")
	}
	if screen.PollEvent() != nil {
		t.Fatal("quit did not finalize screen")
	}
}

func TestDashboardCtrlCFromSearchConfirms(t *testing.T) {
	d, _ := dashboard(t)
	press(d, tcell.KeyRune, '/')
	text(d, "light")
	press(d, tcell.KeyCtrlC, 0)
	if d.modal == nil {
		t.Fatal("Ctrl-C must confirm, including search focus")
	}
	press(d, tcell.KeyEscape, 0)
	if d.modal != nil || d.app.GetFocus() != d.search || d.store.Snapshot().Revision != 1 {
		t.Fatal("cancel failed to restore search without mutation")
	}
}

func TestDashboardAirconRangeBeforeMutation(t *testing.T) {
	for _, entry := range []struct {
		value string
		want  int
		valid bool
	}{{"0", 0, true}, {"50", 50, true}, {"51", 24, false}, {"253", 24, false}} {
		t.Run(entry.value, func(t *testing.T) {
			d, _ := dashboard(t)
			press(d, tcell.KeyDown, 0)
			press(d, tcell.KeyEnter, 0)
			press(d, tcell.KeyEnter, 0)
			form := d.modal.(*tview.Form)
			before := d.store.Snapshot().Revision
			form.GetFormItem(0).(*tview.Checkbox).SetChecked(true)
			form.GetFormItem(2).(*tview.InputField).SetText(entry.value)
			form.GetButton(0).InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), d.focus)
			after := d.store.Snapshot()
			if after.Devices[1].Target != entry.want {
				t.Fatal(after.Devices[1])
			}
			if !entry.valid && (after.Revision != before || d.modal == nil) {
				t.Fatal("invalid form partially mutated state")
			}
		})
	}
}
