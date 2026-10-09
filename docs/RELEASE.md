# v1.1.0

## Changes and migration

Normal startup now enables ECHONET Lite IPv4 UDP and multicast on one selected network interface. `make preview`, `make tui`, and the plain interface use the same selection policy. One usable interface is selected automatically; several interfaces open a keyboard picker showing names and IPv4 addresses. Up/Down selects, Enter starts, and Esc cancels. Loopback, down, IPv6-only and non-multicast interfaces are excluded. Multiple IPv4 addresses use the first address in sorted order.

Use `--offline` when migrating scripts or workflows that relied on the v1.0.0 offline default. `--demo`, `make demo`, `make export` and `--help` remain independent of network interfaces. Headless startup with several candidates requires `--interface NAME`; no candidate fails with an explanation. Existing explicit `--udp`, `--allow-lan` and `--multicast-interface` flags remain available.

The selected interface receives on UDP 3610 and joins 224.0.23.0:3610. Startup D5 and required state-change notifications are enabled. HTTP stays on loopback. Linux multicast reception is limited to the socket's own membership. The pinned uecho-go dependency is unchanged.

## Verification and remaining limits

Mac and Ubuntu CI cover formatting, vet, tests/race, darwin/arm64 and linux/arm64 builds, offline demo/export/help, injected interface selection and transport failure/cleanup. Ubuntu additionally verifies loopback controller-to-display and IPv4 multicast in isolated network namespaces. Offline TUI exit and terminal restoration were checked in a PTY.

The real-environment check is limited to M6 controller -> M4 simulator lighting ON/OFF, with SetC followed by Get returning the matching status. This is not a claim of physical-appliance interoperability, physical Raspberry Pi execution, AC/sensor interoperability, or exhaustive macOS/multiple-NIC multicast verification. No new LAN test is performed during release preparation.

Multicast sockets retain SO_REUSEADDR: another reuse-enabled process can share UDP 3610. An ordinary exclusive bind conflict fails startup; sharing between reuse-enabled processes is not prevented. Interface failure/hotplug recovery, IPv6, persistence, every optional MRA function, signing/notarization and hardware drivers are not implemented or verified. Virtual device state is volatile.

## Binary packages

Assets are `uecho-simulator-v1.1.0-darwin-arm64.tar.gz`, `uecho-simulator-v1.1.0-linux-arm64.tar.gz`, and an archive-level `SHA256SUMS`. Each archive includes the binary, VERSION, BUILD-INFO.txt with exact source SHA/toolchain/dependency, RUNNING.md, README.md, LICENSE, docs/images, THIRD_PARTY_NOTICES.md and internal file-level SHA256SUMS.

Build from the exact merged release commit with GOWORK=off, CGO_ENABLED=0, -trimpath, -buildvcs=false and -ldflags='-s -w'. Verify archive and internal checksums, executable target formats, version/build metadata, and native Mac offline demo/export. Cross-compilation alone does not verify Linux execution. The GitHub Release records the final source SHA, CI and package verification.
