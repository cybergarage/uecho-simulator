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
	"sync"

	"github.com/cybergarage/uecho-simulator/internal/display"
	"github.com/cybergarage/uecho-simulator/internal/model"
	livepreview "github.com/cybergarage/uecho-simulator/internal/preview"
	"github.com/cybergarage/uecho-simulator/internal/tui"
	"github.com/cybergarage/uecho-simulator/internal/wire"
	"github.com/gdamore/tcell/v2"
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
	displayAddress := flags.String("display", "", "read-only browser display on literal loopback address, e.g. 127.0.0.1:8080")
	allowLAN := flags.Bool("allow-lan", false, "explicitly permit UDP binding on a local LAN IPv4 address; no discovery unless --multicast-interface is set")
	multicast := flags.String("multicast-interface", "", "explicit IPv4 ECHONET discovery/notifications on a named test-network interface")
	udp := flags.String("udp", "", "opt in to loopback UDP, e.g. 127.0.0.1:3610")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if *multicast != "" && (!*allowLAN || *displayAddress == "" || *udp == "") {
		return fmt.Errorf("--multicast-interface requires --display, --udp and --allow-lan")
	}
	if *allowLAN && (*displayAddress == "" || *udp == "") {
		return fmt.Errorf("--allow-lan requires --display and --udp")
	}
	if *displayAddress != "" {
		if *plain || *demo || *image != "" {
			return fmt.Errorf("--display cannot combine with --plain, --demo or --preview (SVG export)")
		}
		return runDisplay(*displayAddress, *udp, *allowLAN, *multicast)
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
	var udpWorker sync.WaitGroup
	defer func() { stop(); udpWorker.Wait() }()
	transportFailures := make(chan error, 1)
	if *udp != "" {
		server, err := wire.Listen(*udp, e)
		if err != nil {
			return err
		}
		defer func() { stop(); server.Close() }()
		fmt.Fprintf(out, "Explicit loopback UDP: %s (unicast only; notifications to active peers)\n", server.Address())
		udpWorker.Add(1)
		go func() {
			defer udpWorker.Done()
			if err := server.Serve(ctx); err != nil && ctx.Err() == nil {
				transportFailures <- err
				stop()
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
			select {
			case err := <-transportFailures:
				return err
			default:
				return nil
			}
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

func runDisplay(address, udpAddress string, allowLAN bool, multicastInterface string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	store := model.New()
	engine := wire.New(store)
	listener, err := livepreview.Listen(address)
	if err != nil {
		return err
	}
	var workers sync.WaitGroup
	defer func() { cancel(); _ = listener.Close(); workers.Wait() }()
	page := &livepreview.Server{Store: store}
	failures := make(chan error, 2)
	if udpAddress != "" {
		udp, err := wire.ListenConfigured(udpAddress, engine, allowLAN, multicastInterface)
		if err != nil {
			return err
		}
		defer udp.Close()
		page.UDPAddress = udp.Address()
		workers.Add(1)
		go func() {
			defer workers.Done()
			err := udp.Serve(ctx)
			if err != nil && ctx.Err() == nil {
				failures <- err
				cancel()
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		err := livepreview.Serve(ctx, listener, page.Handler())
		if err != nil && ctx.Err() == nil {
			failures <- err
			cancel()
		}
	}()
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	err = livepreview.Terminal(ctx, screen, store, "http://"+listener.Addr().String(), "room.svg")
	cancel()
	select {
	case failure := <-failures:
		return failure
	default:
		return err
	}
}
