package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/cybergarage/uecho-simulator/internal/display"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/tui"
	"github.com/cybergarage/uecho-simulator/internal/wire"
)

func preview(path string, s model.Snapshot) error {
	if path == "" {
		return nil
	}
	f, err := (display.SVG{}).Render(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, f.Data, 0644)
}
func run(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("uecho-simulator", flag.ContinueOnError)
	flags.SetOutput(out)
	demo := flags.Bool("demo", false, "run the evening scenario and exit without networking")
	plain := flags.Bool("plain", false, "use the plain line interface for piping (default: full-screen TUI)")
	image := flags.String("preview", "", "write an 800x480 monochrome SVG after state changes")
	udp := flags.String("udp", "", "opt in to loopback UDP, e.g. 127.0.0.1:3610")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if *udp != "" && !*plain {
		return fmt.Errorf("optional UDP requires --plain; the full-screen TUI is offline")
	}
	if *demo && *udp != "" {
		return fmt.Errorf("demo cannot enable UDP")
	}
	s := model.New()
	e := wire.New(s)
	u := tui.UI{Store: s, Engine: e, Plain: *plain}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *udp != "" {
		server, err := wire.Listen(*udp, e)
		if err != nil {
			return err
		}
		defer func() { stop(); server.Close() }()
		fmt.Fprintf(out, "Explicit loopback UDP: %s (no discovery or advertisement)\n", server.Address())
		go func() {
			if err := server.Serve(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}()
	} else if *demo || *plain {
		fmt.Fprintln(out, "OFFLINE: frames in memory; no network or hardware")
	}
	if *demo {
		if err := u.Demo(); err != nil {
			return err
		}
		u.Draw(out)
		return preview(*image, s.Snapshot())
	}
	if !*plain {
		if err := preview(*image, s.Snapshot()); err != nil {
			return err
		}
		return tui.NewDashboard(s, e, func(s model.Snapshot) error { return preview(*image, s) }).Run(ctx)
	}
	u.Draw(out)
	if err := preview(*image, s.Snapshot()); err != nil {
		return err
	}
	type inputLine struct {
		text string
		err  error
		done bool
	}
	lines := make(chan inputLine)
	go func() {
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			select {
			case lines <- inputLine{text: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case lines <- inputLine{err: scanner.Err(), done: true}:
		case <-ctx.Done():
		}
	}()
	for {
		fmt.Fprint(out, "> ")
		var line inputLine
		select {
		case <-ctx.Done():
			return nil
		case line = <-lines:
		}
		if line.done {
			return line.err
		}
		quit, err := u.Command(line.text, out)
		if err != nil {
			fmt.Fprintf(out, "ERROR: %v\n", err)
			continue
		}
		if quit {
			return nil
		}
		command := strings.TrimSpace(line.text)
		if command != "events" && command != "help" {
			u.Draw(out)
		}
		if err := preview(*image, s.Snapshot()); err != nil {
			return err
		}
	}
}
func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
