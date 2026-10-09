GO ?= go

.PHONY: help tui preview export demo

help:
	@printf '%s\n' \
	  'make tui     - Start the full-screen offline TUI; update room.svg' \
	  'make preview - Keep a read-only browser display running at localhost:8080' \
	  'make export  - Generate room.svg from the offline evening demo, then exit' \
	  'make demo    - Run the offline evening demo in plain text, then exit' \
	  'make help    - Show this target list'

tui:
	GOWORK=off $(GO) run ./cmd/uecho-simulator --preview room.svg

preview:
	GOWORK=off $(GO) run ./cmd/uecho-simulator --display 127.0.0.1:8080 $(PREVIEW_ARGS)

export:
	GOWORK=off $(GO) run ./cmd/uecho-simulator --demo --plain --preview room.svg

demo:
	GOWORK=off $(GO) run ./cmd/uecho-simulator --demo --plain
