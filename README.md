# uecho-simulator

A small ECHONET Lite room simulator powered by [uecho-go](https://github.com/cybergarage/uecho-go). It runs on Mac/Linux and Raspberry Pi 4/5 without extra equipment. Raspberry Pi Pico is outside this project's scope.

The default mode is fully offline: requests are encoded, decoded and handled in memory. It opens no sockets, discovers no devices and sends no advertisements. The prototype implements explicit small profiles, not complete ECHONET Lite/MRA compliance.

## Quick start

Install Go 1.25 or later and Make, then:

```sh
git clone git@github.com:cybergarage/uecho-simulator.git
cd uecho-simulator
go mod download
make help
make tui
```

`make tui` starts the interactive full-screen UI. `make preview` keeps a read-only browser display running; `make export` generates room.svg and exits; `make demo` prints the evening scenario and exits. `make help` lists all five targets. Go commands use the pinned dependency with `GOWORK=off`. Generated room.svg is ignored by Git.

## Read-only browser preview

```sh
make preview
```

Open **http://127.0.0.1:8080/** in a browser on the same Mac or Pi. The bold English display is designed for 800×480 and has no device controls. It shows initial simulated values until controller input arrives. Preview stays running even without incoming events; it does not read line commands or exit at stdin EOF. A real terminal is required for its keys.

These keys work in the **launching terminal**, not the browser:

| Key | Action |
| --- | --- |
| `q` / Ctrl-C | Stop HTTP/UDP, close streams and restore terminal mode |
| `?` | Show key help |
| `r` | Re-send the current model to all displays without changing devices |
| `s` | Save the current state as room.svg using the existing static scene adapter |

The browser receives snapshots through Server-Sent Events when the shared model changes, rather than polling. Reconnecting displays immediately receive the latest state. `DISPLAY DISCONNECTED` retains last values while reconnecting. `DISPLAY LIVE` describes the browser stream, **not controller connectivity**. UDP is connectionless: the display reports whether a frame has arrived and the last RX time; it never claims a controller is connected. The state timestamp changes only on a state mutation. Read requests and invalid frames can update RX without changing state. No artificial temperature evolution runs.

![Read-only browser display](docs/images/preview.png)

This screenshot is from the actual local browser. The supplied visual reference could not be downloaded (Library returned HTTP 403); this design implements the requested bold English style without claiming to match that unseen image.

### Explicit controller input

Preview starts with controller input disabled. To accept a controller on this machine, opt in to loopback unicast UDP:

```sh
make preview PREVIEW_ARGS='--udp 127.0.0.1:3610'
```

For a later test from a separate LAN controller, first confirm the simulator's own local IPv4 address, that the interface belongs to the intended isolated/test network, that port 3610 is available, and that the controller supports manually addressed unicast Get/SetC for the listed EOJs. Replace `192.168.1.50` with that confirmed address:

```sh
make preview PREVIEW_ARGS='--udp 192.168.1.50:3610 --allow-lan'
```

Only this explicit opt-in permits a local unicast LAN UDP bind. HTTP always remains loopback: open the browser on the simulator machine. There is no discovery, multicast membership, advertisement, INF or node-profile service; discovery-dependent controllers will not find this prototype. SetC updates the same model shown in the display; Get reads it. Sensor temperature remains read only. This task tested loopback and injected events only, not household LAN or physical devices.

A browser has no mutation or shutdown endpoint. The LAN UDP prototype has no authentication, so use only the intended test network and stop it from the launching terminal afterward. No firewall or permissions are changed by the program.

## Full-screen terminal UI

The default command starts a selection-based [tview](https://github.com/rivo/tview)/[tcell](https://github.com/gdamore/tcell) dashboard. It shows devices, selected state/properties, available actions and an explicitly labelled offline simulated-event log. No command string is required for device operation.

![Selection dashboard](docs/images/tui.png)

[Device form](docs/images/tui-form.png) · [Compact layout](docs/images/tui-compact.png). These images come from real tcell widget drawing in the simulation-screen tests, rather than a visual mockup.

| Key | Action |
| --- | --- |
| Tab / Shift-Tab | Cycle Devices → Actions → Events; cyan border marks focus |
| Up / Down | Select a device/action or scroll events |
| Enter | From Devices, focus Actions; from Actions, open a form/menu |
| `/` | Search device name, kind or EOJ; Enter returns to Devices |
| Esc | Cancel a dialog without applying; outside dialogs clear search / focus Devices |
| `s` | Open evening-scenario confirmation (Cancel is selected first) |
| `?` | Open keyboard help (arrows scroll; Tab reaches Close) |
| `q` / Ctrl-C | Open quit confirmation; Esc cancels it |

In forms, Tab/Shift-Tab moves between fields and buttons, Space toggles Power, and Enter opens/selects a mode dropdown or activates Apply/Cancel. Brightness and AC target use numeric fields. Temperature sensor EPC E0 remains read only: its form changes a **local ambient scenario input**, not a protocol write. Input validation happens before any state mutation.

`demo` applies the deterministic evening scenario: light on at 75%, air conditioner on in cool mode at 24°C, room temperature 26.5°C. Use `s` and confirm it in the TUI, or use `--demo` for a non-interactive run. Temperature is injected; no thermodynamic model runs in the background.

At widths below 85 columns or heights below 26 rows, the UI stacks Devices and Actions and reduces the log pane. The wide property pane is available through **View state / supported properties**. Selection and open forms survive resize; dialogs are constrained to terminal bounds. Use at least 45×18 (85×26 or larger recommended). Smaller screens remain cancellable but may clip content. Mouse navigation is currently disabled. Exit through the confirmation dialog; tcell restores the terminal's normal screen/input mode.

## Non-interactive / plain mode

The offline demo and SVG export remain available for automation:

```sh
make demo
make export
```

`--plain` retains the earlier line interface for piped input, rather than opening a full-screen UI:

```sh
printf 'light on\nac cool\ntemp 26.5\nquit\n' | go run ./cmd/uecho-simulator --plain
```

In-memory frames are labelled `SIM-RX`/`SIM-TX`; these are simulated events, not socket captures. The standard TUI is offline only. Optional unicast UDP is available through an explicit preview opt-in as above, or the legacy `--udp ... --plain` loopback mode.

## Implemented profiles

| Device | EOJ | Properties beyond common status/maps |
| --- | --- | --- |
| General lighting | 029001 | B0: brightness 0–100% |
| Home air conditioner | 013001 | B0: cool/heat/fan; B3: target 16–30°C; BB: room temperature |
| Temperature sensor | 001101 | E0: signed big-endian temperature in 0.1°C, read only |

Common EPCs: 80 operation status, 88 fault status, 8A experimental manufacturer value FFFFFF, 9D empty announcement map, 9E Set map, 9F Get map. Maps describe only implemented properties and currently fit Format 1. No full MRA object/property set is inherited. Get and SetC are supported, including error responses and multiple-property frames. Rejected individual writes do not change their state; a multi-property request may apply valid writes before returning an error for another property.

Not implemented: node profile/discovery, INF notifications, SetI/SetGet, complete MRA conformance, device persistence, physical appliances, Sense HAT/e-paper drivers. The sensor always reports powered on and cannot be written over the protocol.

## Optional loopback transport

Networking is disabled unless explicitly requested:

```sh
go run ./cmd/uecho-simulator --udp 127.0.0.1:3610 --plain
```

This prototype accepts only a literal IPv4 loopback bind. Wildcard, multicast, hostname and LAN addresses are refused. It responds only to received requests and does not scan or advertise. UDP mode is excluded from `--demo`. The legacy plain interface redraws on local commands; use the browser preview for event-driven updates from UDP. Exit with `quit` or Ctrl-C.

## Raspberry Pi 4/5

Use a 64-bit Raspberry Pi OS/Linux installation and a Go 1.25+ arm64 toolchain. No GPIO, SPI, HAT configuration or privilege changes are needed for the offline demo. The repository is private, so use an existing authorized GitHub credential for checkout.

On a Mac or Linux development machine, build the portable binary:

```sh
mkdir -p bin
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/uecho-simulator-linux-arm64 ./cmd/uecho-simulator
```

After transferring it through your existing authorized workflow, run `./uecho-simulator-linux-arm64 --demo --plain --preview room.svg` on the Pi. Physical Pi execution is not yet verified; the arm64 binary is cross-built in CI. No additional screen or sensor is required.

## Architecture and development

- `internal/model`: synchronized virtual state and detached snapshots; no network or hardware.
- `internal/wire`: uecho-go frame handling plus optional transport.
- `internal/tui`: full-screen selection UI, legacy plain interface and deterministic scenario.
- `internal/preview`: read-only loopback HTTP/SSE display and terminal lifecycle keys.
- `internal/display`: `Adapter.Render(snapshot) -> Frame`; SVG is the initial adapter. Future Sense HAT rev2/e-paper adapters own pixel conversion, refresh intervals and batching without changing model/protocol code.
- `cmd/uecho-simulator`: explicit runtime modes and lifecycle.

uecho-go is pinned to the merged property-map fix; no local replace is committed. To develop both libraries together, create an untracked workspace outside the repositories:

```sh
cd ~/Src
go work init ./uecho-go ./uecho-simulator
```

CI sets `GOWORK=off` to check the pinned dependency rather than a local checkout. Remove/disable your local workspace when validating the released dependency.

```sh
./scripts/check.sh
```

Checks include formatting, vet, tests/race (including tcell keyboard events, form apply/cancel, search targeting, resize and screen finalization), darwin/arm64 and linux/arm64 builds, and an offline SVG demo. Default checks use injected frames and deny network access. The separate opt-in `SIMULATOR_LOOPBACK_TEST=1 GOWORK=off go test -race ./internal/preview -run TestLoopbackControllerToDisplay` binds only 127.0.0.1 and verifies UDP request/response, model propagation, event delivery and shutdown. CI denies network access during checks after downloading dependencies. The initial work-in-progress archive remains preserved separately; no source-library checkout was modified during migration.

BSD 3-Clause; see LICENSE and THIRD_PARTY_NOTICES.md.

## UI references

Navigation and visible key hints were informed by the official [k9s README](https://github.com/derailed/k9s) and [command guide](https://k9scli.io/topics/commands/). Widget composition follows the official [tview README](https://github.com/rivo/tview) and [form example](https://github.com/rivo/tview/blob/master/demos/form/main.go). No Kubernetes functionality or k9s code/assets are included. tview/tcell and transitive runtime versions are fixed in go.mod/go.sum; their upstream notices are in THIRD_PARTY_NOTICES.md.
