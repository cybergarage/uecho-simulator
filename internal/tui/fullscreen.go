package tui

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/wire"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Dashboard owns widgets on the tview event loop. It has no socket or driver.
type Dashboard struct {
	Network                             string
	app                                 *tview.Application
	pages                               *tview.Pages
	layout, body, right                 *tview.Flex
	table                               *tview.Table
	details, logs, header, help, status *tview.TextView
	search                              *tview.InputField
	actions                             *tview.List
	store                               *model.Store
	engine                              *wire.Engine
	updated                             func(model.Snapshot) error
	visible                             []model.Device
	selected                            uint32
	modal                               tview.Primitive
	prior                               tview.Primitive
	width, height                       int
	compact                             bool
}

func NewDashboard(s *model.Store, e *wire.Engine, updated func(model.Snapshot) error) *Dashboard {
	d := &Dashboard{app: tview.NewApplication(), store: s, engine: e, updated: updated, selected: model.Light}
	d.table = tview.NewTable().SetSelectable(true, false).SetFixed(1, 0).SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorDarkCyan).Foreground(tcell.ColorWhite))
	d.table.SetBorder(true).SetTitle(" Devices ")
	d.details = tview.NewTextView().SetDynamicColors(true)
	d.details.SetBorder(true).SetTitle(" State / supported properties ")
	d.actions = tview.NewList().ShowSecondaryText(false)
	d.actions.SetBorder(true).SetTitle(" Actions - Enter ")
	d.logs = tview.NewTextView().SetDynamicColors(false).SetWrap(false).SetScrollable(true)
	d.logs.SetBorder(true).SetTitle(" Events - simulated frames and UDP ")
	d.header = tview.NewTextView().SetDynamicColors(true)
	d.help = tview.NewTextView().SetText("Tab/Shift-Tab focus | Arrows select | Enter act\n/ search | s scenario | ? help | q quit | Esc cancel")
	d.status = tview.NewTextView().SetDynamicColors(true).SetText("[gray]Ready: virtual devices[-]")
	d.search = tview.NewInputField().SetLabel(" / Search: ").SetFieldWidth(0)
	d.search.SetChangedFunc(func(string) { d.refresh() })
	d.search.SetDoneFunc(func(tcell.Key) { d.focus(d.table) })
	d.table.SetSelectionChangedFunc(func(row, _ int) {
		if row > 0 && row <= len(d.visible) {
			d.selected = d.visible[row-1].EOJ
			d.refreshDetails()
		}
	})
	d.table.SetSelectedFunc(func(int, int) { d.focus(d.actions) })
	d.body = tview.NewFlex()
	d.right = tview.NewFlex().SetDirection(tview.FlexRow)
	d.layout = tview.NewFlex().SetDirection(tview.FlexRow)
	d.pages = tview.NewPages().AddPage("main", d.layout, true, true)
	d.app.SetRoot(d.pages, true).EnableMouse(false).EnablePaste(true).SetInputCapture(d.capture)
	for _, box := range []*tview.Box{d.table.Box, d.actions.Box, d.logs.Box} {
		box.SetBorderColor(tcell.ColorGray)
		box.SetFocusFunc(func() { box.SetBorderColor(tcell.ColorAqua) })
		box.SetBlurFunc(func() { box.SetBorderColor(tcell.ColorGray) })
	}
	d.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		w, h := screen.Size()
		d.resize(w, h)
		return false
	})
	d.refresh()
	d.focus(d.table)
	return d
}

func (d *Dashboard) Run(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			d.app.Stop()
		case <-done:
		}
	}()
	defer close(done)
	// tview finalizes its screen on Stop and on a Run error/panic.
	return d.app.Run()
}

func (d *Dashboard) resize(w, h int) {
	if d.width == w && d.height == h {
		return
	}
	wasSmall := d.width < 45 || d.height < 18
	d.width, d.height = w, h
	d.compact = w < 85 || h < 26
	d.layout.Clear()
	d.body.Clear()
	d.right.Clear()
	if d.compact {
		d.body.SetDirection(tview.FlexRow).AddItem(d.table, 5, 0, false).AddItem(d.actions, 0, 1, false)
	} else {
		d.right.AddItem(d.details, 10, 0, false).AddItem(d.actions, 0, 1, false)
		d.body.SetDirection(tview.FlexColumn).AddItem(d.table, 40, 0, false).AddItem(d.right, 0, 1, false)
	}
	logHeight := 8
	if d.compact {
		logHeight = 4
	}
	d.layout.AddItem(d.header, 1, 0, false).AddItem(d.search, 1, 0, false).AddItem(d.body, 0, 1, true).AddItem(d.logs, logHeight, 0, false).AddItem(d.help, 2, 0, false).AddItem(d.status, 1, 0, false)
	if w < 45 || h < 18 {
		d.status.SetText("[yellow]Use >=45x18; Tab/Enter/Esc still work[-]")
	} else if wasSmall {
		d.status.SetText("[gray]Terminal resized; ready[-]")
	}
}

func (d *Dashboard) focus(p tview.Primitive) { d.app.SetFocus(p) }
func (d *Dashboard) capture(event *tcell.EventKey) *tcell.EventKey {
	if d.modal != nil {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlC {
			d.closeModal()
			return nil
		}
		return event
	}
	if event.Key() == tcell.KeyCtrlC {
		d.quit()
		return nil
	}
	if d.app.GetFocus() == d.search {
		if event.Key() == tcell.KeyEscape {
			d.search.SetText("")
			d.focus(d.table)
			return nil
		}
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			d.focus(d.table)
			return nil
		}
		return event
	}
	switch event.Key() {
	case tcell.KeyTab, tcell.KeyBacktab:
		panes := []tview.Primitive{d.table, d.actions, d.logs}
		n := 0
		for i, p := range panes {
			if d.app.GetFocus() == p {
				n = i
			}
		}
		if event.Key() == tcell.KeyBacktab {
			n = (n + len(panes) - 1) % len(panes)
		} else {
			n = (n + 1) % len(panes)
		}
		d.focus(panes[n])
		return nil
	case tcell.KeyEscape:
		d.search.SetText("")
		d.focus(d.table)
		return nil
	}
	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case '/':
			d.focus(d.search)
			return nil
		case 's':
			d.scenario()
			return nil
		case 'q':
			d.quit()
			return nil
		case '?':
			d.message("Keyboard help", "Tab / Shift-Tab: cycle devices, actions, logs\nArrows: select or scroll; Enter: open / apply\n/: filter device name, EOJ or kind\nEsc: cancel dialog without changing state / clear filter\ns: evening scenario confirmation; q: quit confirmation\nForms: Tab to next field/button; Space toggles a checkbox.\nSIM events are in-memory frames; UDP events are network traffic.")
			return nil
		}
	}
	return event
}

func (d *Dashboard) refresh() {
	snap := d.store.Snapshot()
	filter := strings.ToLower(d.search.GetText())
	old := d.selected
	d.visible = nil
	for _, device := range snap.Devices {
		if strings.Contains(strings.ToLower(fmt.Sprintf("%s %s %06X", device.Name, device.Kind, device.EOJ)), filter) {
			d.visible = append(d.visible, device)
		}
	}
	d.table.Clear()
	for col, title := range []string{"DEVICE", "EOJ", "STATE"} {
		d.table.SetCell(0, col, tview.NewTableCell(title).SetSelectable(false).SetTextColor(tcell.ColorAqua))
	}
	row := 1
	for i, device := range d.visible {
		state := "OFF"
		if device.Power {
			state = "ON"
		}
		for col, value := range []string{device.Name, fmt.Sprintf("%06X", device.EOJ), state} {
			d.table.SetCell(i+1, col, tview.NewTableCell(value))
		}
		if device.EOJ == old {
			row = i + 1
		}
	}
	d.selected = 0
	if len(d.visible) > 0 {
		d.selected = d.visible[row-1].EOJ
		d.table.Select(row, 0)
	}
	mode := "OFFLINE / in-memory"
	if d.Network != "" {
		mode = "NETWORK / " + d.Network
	}
	d.header.SetText(fmt.Sprintf("[aqua] UECHO[-] | [green]%s[-] | %.1f C | rev %d", mode, float64(snap.AmbientTenths)/10, snap.Revision))
	var log strings.Builder
	for _, event := range snap.Events {
		fmt.Fprintf(&log, "%03d %s %-7s %-7s %s", event.Sequence, event.Time, event.Kind, event.Source, event.Message)
		if event.Frame != "" {
			fmt.Fprintf(&log, " [%s]", event.Frame)
		}
		log.WriteByte('\n')
	}
	d.logs.SetText(log.String()).ScrollToEnd()
	d.refreshDetails()
}
func (d *Dashboard) device() (model.Device, bool) {
	for _, device := range d.store.Snapshot().Devices {
		if device.EOJ == d.selected {
			return device, true
		}
	}
	return model.Device{}, false
}
func (d *Dashboard) refreshDetails() {
	d.actions.Clear()
	device, ok := d.device()
	if !ok {
		d.details.SetText("No matching devices. Esc clears the search.")
		d.actions.AddItem("Clear search", "", 0, func() { d.search.SetText(""); d.focus(d.table) })
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[aqua]%s[-] / EOJ %06X\n", tview.Escape(device.Name), device.EOJ)
	switch device.Kind {
	case "light":
		fmt.Fprintf(&b, "Power %t | Brightness %d%%\n", device.Power, device.Level)
	case "aircon":
		fmt.Fprintf(&b, "Power %t | %s | Target %s\n", device.Power, device.Mode, device.TargetLabel())
	case "sensor":
		fmt.Fprintln(&b, "E0 is read only; ambient is a local scenario input.")
	}
	for _, p := range device.Properties {
		access := "GET"
		if p.Writable {
			access = "GET/SET"
		}
		fmt.Fprintf(&b, "%02X %-7s %s: %s\n", p.EPC, access, tview.Escape(p.Name), p.Value)
	}
	d.details.SetText(b.String())
	d.actions.AddItem("Edit "+device.Name, "", 0, func() { d.edit() })
	d.actions.AddItem("View state / supported properties...", "", 0, func() { d.message(device.Name, d.details.GetText(true)) })
	d.actions.AddItem("Read supported properties (simulated)", "", 0, func() {
		for _, p := range device.Properties {
			if _, err := d.engine.Request(device.EOJ, p.EPC, nil, false, "tui"); err != nil {
				d.result(err)
				return
			}
		}
		d.result(nil)
	})
	d.actions.AddItem("Evening scenario...", "", 0, d.scenario)
	d.actions.AddItem("Keyboard help...", "", 0, func() { d.capture(tcell.NewEventKey(tcell.KeyRune, '?', 0)) })
	d.actions.AddItem("Quit...", "", 0, d.quit)
	d.actions.SetCurrentItem(0)
}
func (d *Dashboard) result(err error) {
	if err != nil {
		d.status.SetText("[red]" + tview.Escape(err.Error()) + "[-]")
		return
	}
	d.refresh()
	if d.updated != nil {
		if err := d.updated(d.store.Snapshot()); err != nil {
			d.status.SetText("[red]Preview: " + tview.Escape(err.Error()) + "[-]")
			return
		}
	}
	d.status.SetText("[green]Applied locally; no network traffic[-]")
}

// centered keeps dialogs inside the current terminal bounds, including resize.
type centered struct {
	*tview.Box
	child         tview.Primitive
	width, height int
}

func (c *centered) Focus(delegate func(tview.Primitive)) { c.child.Focus(delegate) }
func (c *centered) HasFocus() bool                       { return c.child.HasFocus() }
func (c *centered) Blur()                                { c.child.Blur() }
func (c *centered) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return c.child.InputHandler()
}
func (c *centered) PasteHandler() func(string, func(tview.Primitive)) { return c.child.PasteHandler() }
func (c *centered) Draw(screen tcell.Screen) {
	x, y, w, h := c.GetInnerRect()
	w = min(w, c.width)
	h = min(h, c.height)
	rw, rh := screen.Size()
	c.child.SetRect(x+(rw-w)/2, y+(rh-h)/2, w, h)
	c.child.Draw(screen)
}
func (d *Dashboard) open(p tview.Primitive, w, h int) {
	d.prior = d.app.GetFocus()
	d.modal = p
	d.pages.AddPage("dialog", &centered{tview.NewBox(), p, w, h}, true, true)
	d.focus(p)
}
func (d *Dashboard) closeModal() {
	d.pages.RemovePage("dialog")
	d.modal = nil
	p := d.prior
	d.prior = nil
	if p == nil {
		p = d.table
	}
	d.focus(p)
	d.status.SetText("[gray]Dialog closed; Esc cancels without applying[-]")
}
func (d *Dashboard) message(title, text string) {
	body := tview.NewTextView().SetText(text).SetScrollable(true).SetWrap(true)
	buttons := tview.NewForm().AddButton("Close", d.closeModal)
	body.SetDoneFunc(func(tcell.Key) { d.focus(buttons) })
	buttons.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			d.focus(body)
			return nil
		}
		return event
	})
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(body, 0, 1, true).AddItem(buttons, 3, 0, false)
	box.SetBorder(true).SetTitle(" " + title + " ")
	d.open(box, 70, 20)
}
func (d *Dashboard) confirm(text string, apply func()) {
	body := tview.NewTextView().SetText(text).SetWrap(true)
	buttons := tview.NewForm().AddButton("Cancel", d.closeModal).AddButton("Confirm", func() { d.closeModal(); apply() })
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(body, 0, 1, false).AddItem(buttons, 3, 0, true)
	box.SetBorder(true).SetTitle(" Confirm - Esc cancels ")
	d.open(box, 58, 12)
}
func (d *Dashboard) scenario() {
	d.confirm("Apply the evening scenario to virtual devices?\nLight 75%, AC cool / 24 C, ambient 26.5 C.\nChanges apply to virtual devices; network mode sends notifications.", func() { u := UI{Store: d.store, Engine: d.engine}; d.result(u.Demo()) })
}
func (d *Dashboard) quit() {
	d.confirm("Exit the simulator?\nVirtual state is not persisted.", d.app.Stop)
}
func (d *Dashboard) edit() {
	device, ok := d.device()
	if !ok {
		return
	}
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" Edit " + device.Name + " - Esc cancels ")
	power := device.Power
	mode := device.Mode
	var field *tview.InputField
	if device.Kind != "sensor" {
		form.AddCheckbox("Power", power, func(v bool) { power = v })
	}
	switch device.Kind {
	case "light":
		form.AddInputField("Brightness 0..100", strconv.Itoa(device.Level), 8, tview.InputFieldInteger, nil)
	case "aircon":
		options := []string{"cool", "heat", "fan", "auto", "dry", "other"}
		index := 0
		for i, v := range options {
			if v == mode {
				index = i
			}
		}
		target := strconv.Itoa(device.Target)
		form.AddDropDown("Mode", options, index, func(v string, _ int) { mode = v }).AddInputField("Target 0..50 C", target, 8, tview.InputFieldInteger, nil)
	case "sensor":
		form.AddTextView("Read only", "E0 cannot be written. This edits local ambient input.", 44, 2, false, false).AddInputField("Ambient -20..50 C", fmt.Sprintf("%.1f", float64(d.store.Snapshot().AmbientTenths)/10), 10, tview.InputFieldFloat, nil)
	}
	field = form.GetFormItem(form.GetFormItemCount() - 1).(*tview.InputField)
	form.AddButton("Apply", func() {
		value, err := strconv.ParseFloat(field.GetText(), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			d.status.SetText("[red]Enter a finite number[-]")
			return
		}
		switch device.Kind {
		case "light":
			if value < 0 || value > 100 || value != math.Trunc(value) {
				d.status.SetText("[red]Brightness must be an integer 0..100[-]")
				return
			}
		case "aircon":
			if value < 0 || value > 50 || value != math.Trunc(value) {
				d.status.SetText("[red]Target must be an integer 0..50[-]")
				return
			}
		case "sensor":
			if value < -20 || value > 50 || math.Abs(value*10-math.Round(value*10)) > 1e-9 {
				d.status.SetText("[red]Ambient must be -20..50 in tenths[-]")
				return
			}
		}
		d.closeModal()
		if device.Kind == "sensor" {
			d.result(d.store.InjectAmbient(int(math.Round(value*10)), "local-tui"))
			return
		}
		write := func(epc, value byte) error {
			_, err := d.engine.Request(device.EOJ, epc, []byte{value}, true, "tui")
			return err
		}
		status := byte(0x31)
		if power {
			status = 0x30
		}
		if err := write(0x80, status); err != nil {
			d.result(err)
			return
		}
		if device.Kind == "light" {
			d.result(write(0xB0, byte(value)))
			return
		}
		if err := write(0xB0, map[string]byte{"other": 0x40, "auto": 0x41, "cool": 0x42, "heat": 0x43, "dry": 0x44, "fan": 0x45}[mode]); err != nil {
			d.result(err)
			return
		}
		d.result(write(0xB3, byte(value)))
	}).AddButton("Cancel", d.closeModal)
	d.open(form, 58, 15)
}
