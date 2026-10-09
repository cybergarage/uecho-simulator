# v1.0.0 preparation

This document describes the proposed first release. It creates no tag or GitHub Release. Merge the device-completion PR and explicitly approve publication before running a release workflow.

## Scope

- Three virtual device instances (general lighting, home AC, temperature sensor) and one node profile, based on ECHONET Lite v1.14 / Appendix Release R rev.4.
- Mandatory properties for these constrained functions, accurate property maps, instance-list discovery and instance 00 handling.
- Get, SetI, SetC, SetGet, INF_REQ, INFC reception, partial-error responses and required state-change INF.
- UDP destination port 3610; explicit IPv4 multicast-interface mode; fully offline default and manual loopback mode.
- Selection TUI, read-only event-driven browser display and 800×480 SVG export.
- AC target range 0–50°C; undefined writes are rejected because automatic temperature control is not implemented.

This is virtual device functionality, not appliance certification or implementation of every optional MRA function. IPv6, persistence, physical Pi, household LAN, multicast on real interfaces and hardware displays are outside the completed verification. General lighting only supports main lighting mode; no automatic/night/color behavior is advertised. Experimental manufacturer and process IDs are used.

## Verification before publication

1. Select the exact merged commit; confirm its macOS and Ubuntu Check jobs pass.
2. Download the pinned dependencies, set `GOWORK=off`, and run `./scripts/check.sh` with the network denied. This checks formatting, vet, normal/race tests, both arm64 builds and offline demo/export.
3. Run the opt-in controller → UDP → shared model → SSE display test only in localhost or a Linux network namespace with loopback enabled. The test receives on port 3610 and sends from an independent ephemeral port.
4. The injected datagram test validates multicast startup D5, D6 discovery, instance 00, fixed-port replies, INF_REQ routing, ordered changes and both-socket shutdown. A separate Ubuntu CI test creates a dummy interface inside an isolated namespace and validates OS multicast membership/startup/discovery/notifications. It refuses to run when other interfaces are present. Neither test validates physical interfaces or macOS multicast.
5. Run each produced binary's `--demo --plain` on its supported host before claiming native execution there. Cross-compilation alone is not Raspberry Pi execution.

Any future physical/network trial must use an explicitly selected isolated test network and existing authorization. No LAN discovery or hardware trials are prerequisites silently performed by this repository's normal checks.

## Proposed release packages

Publish `uecho-simulator-v1.0.0-darwin-arm64.tar.gz`, `uecho-simulator-v1.0.0-linux-arm64.tar.gz`, and `SHA256SUMS`. A package should contain the binary named `uecho-simulator`, README.md, LICENSE, THIRD_PARTY_NOTICES.md, docs (including images and this release note), and BUILD-INFO.txt. Record the exact commit, exact Go patch version, target OS/arch, build command and pinned module version. Exclude go.work, local credentials, generated room.svg and local caches. A source archive may be supplied separately.

For reproducible binaries, use the same exact Go patch version on every rebuild (initial validation: Go 1.25.1), the same commit and pinned go.sum, and:

```sh
export GOWORK=off GOPROXY=off GOTOOLCHAIN=local
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o bin/uecho-simulator-darwin-arm64 ./cmd/uecho-simulator
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o bin/uecho-simulator-linux-arm64 ./cmd/uecho-simulator
shasum -a 256 bin/uecho-simulator-*
```

Build twice in clean source trees and compare binary SHA-256 values. For identical archives, use one fixed GNU tar version, stable filename sorting, the selected commit time as every file's mtime, uid/gid 0, and `gzip -n`; do not assume BSD tar and GNU tar produce identical archives. BUILD-INFO must use fixed commit metadata rather than wall-clock build timestamps. Archive reproducibility and publication remain release-stage checks; no packages have been published by this PR.
