# uecho-simulator

A small ECHONET Lite room simulator powered by [uecho-go](https://github.com/cybergarage/uecho-go). It runs on Mac/Linux and Raspberry Pi 4/5 without extra equipment. Raspberry Pi Pico is outside this project's scope.

The default mode is fully offline: requests are encoded, decoded and handled in memory. It opens no sockets, discovers no devices and sends no advertisements. The prototype implements explicit small profiles, not complete ECHONET Lite/MRA compliance.

## Quick start

Install Go 1.25 or later, then:

```sh
git clone git@github.com:cybergarage/uecho-simulator.git
cd uecho-simulator
git switch feat/offline-room-simulator # draft prototype branch
go mod download
go run ./cmd/uecho-simulator --demo --plain --preview room.svg
go run ./cmd/uecho-simulator --preview room.svg
```

Open room.svg in a browser or image viewer. It is a static 800×480 black/white room and state view. The SVG file updates after local terminal commands; reload the viewer to see the latest state. No web server or hardware refresh loop is included.

The terminal dashboard accepts commands followed by Enter:

```text
light on
light 75
ac on
ac cool
ac 24
temp 26.5
events
demo
quit
```

`--plain` disables terminal clearing for logs/piped input. `demo` and `--demo` apply the deterministic evening scenario: light on at 75%, air conditioner on in cool mode at 24°C, room temperature 26.5°C. Temperature is an injected scenario input; no thermodynamic model runs in the background.

## Implemented profiles

| Device | EOJ | Properties beyond common status/maps |
| --- | --- | --- |
| General lighting | 029001 | B0: brightness 0–100% |
| Home air conditioner | 013001 | B0: cool/heat/fan; B3: target 16–30°C; BB: room temperature |
| Temperature sensor | 001101 | E0: signed big-endian temperature in 0.1°C, read only |

Common EPCs: 80 operation status, 88 fault status, 8A experimental manufacturer value FFFFFF, 9D empty announcement map, 9E Set map, 9F Get map. Maps describe only implemented properties and currently fit Format 1. No full MRA object/property set is inherited. Get and SetC are supported, including error responses and multiple-property frames. Rejected individual writes do not change their state; a multi-property request may apply valid writes before returning an error for another property.

Not implemented: node profile/discovery, INF notifications, SetI/SetGet, complete MRA conformance, device persistence, live web UI, physical appliances, Sense HAT/e-paper drivers or automatic display refresh. The sensor always reports powered on and cannot be written over the protocol.

## Optional loopback transport

Networking is disabled unless explicitly requested:

```sh
go run ./cmd/uecho-simulator --udp 127.0.0.1:3610 --plain
```

This prototype accepts only a literal IPv4 loopback bind. Wildcard, multicast, hostname and LAN addresses are refused. It responds only to received requests and does not scan or advertise. UDP mode is excluded from `--demo`. The optional transport has address-policy tests; actual socket interaction has not been exercised in this task. UDP state changes appear at the next terminal redraw; they do not trigger automatic preview refresh. Exit with `quit` or Ctrl-C.

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
- `internal/tui`: terminal commands and deterministic scenario.
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

Checks include formatting, vet, tests/race, darwin/arm64 and linux/arm64 builds, and an offline SVG demo. Tests use in-memory frames and never call the UDP listener. CI denies network access during checks after downloading dependencies. The initial work-in-progress archive remains preserved separately; no source-library checkout was modified during migration.

BSD 3-Clause; see LICENSE and THIRD_PARTY_NOTICES.md.
