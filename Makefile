GO ?= go

.PHONY: help tui preview demo

help:
	@printf '%s\n' \
	  'make tui     - Start the full-screen offline TUI; update room.svg' \
	  'make preview - Generate room.svg from the offline evening demo, then exit' \
	  'make demo    - Run the offline evening demo in plain text, then exit' \
	  'make help    - Show this target list'

tui:
	GOWORK=off $(GO) run ./cmd/uecho-simulator --preview room.svg

preview:
	GOWORK=off $(GO) run ./cmd/uecho-simulator --demo --plain --preview room.svg

demo:
	GOWORK=off $(GO) run ./cmd/uecho-simulator --demo --plain
