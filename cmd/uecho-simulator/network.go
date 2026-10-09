package main

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/term"
)

type networkInterface struct {
	name      string
	addresses []string
}

var discoverInterfaces = func() ([]networkInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var result []networkInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("interface %s: %w", iface.Name, err)
		}
		candidate := networkInterface{name: iface.Name}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && ip.IsGlobalUnicast() {
				candidate.addresses = append(candidate.addresses, ip.String())
			}
		}
		sort.Strings(candidate.addresses)
		if len(candidate.addresses) > 0 {
			result = append(result, candidate)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, nil
}
var chooseInterface = func(candidates []networkInterface) (networkInterface, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		var names []string
		for _, candidate := range candidates {
			names = append(names, candidate.name+" ("+strings.Join(candidate.addresses, ", ")+")")
		}
		return networkInterface{}, fmt.Errorf("multiple network interfaces: %s; select one with --interface NAME (or use --offline)", strings.Join(names, "; "))
	}
	screen, err := tcell.NewScreen()
	if err != nil {
		return networkInterface{}, err
	}
	return selectInterface(screen, candidates)
}

func selectInterface(screen tcell.Screen, candidates []networkInterface) (networkInterface, error) {
	if err := screen.Init(); err != nil {
		return networkInterface{}, err
	}
	defer screen.Fini()
	selected := 0
	draw := func() {
		screen.Clear()
		lines := []string{"Select network interface", "Up/Down: select   Enter: start   Esc: cancel"}
		for i, c := range candidates {
			prefix := "  "
			if i == selected {
				prefix = "> "
			}
			lines = append(lines, prefix+c.name+"  "+strings.Join(c.addresses, ", "))
		}
		for y, line := range lines {
			for x, r := range []rune(line) {
				screen.SetContent(x, y, r, nil, tcell.StyleDefault)
			}
		}
		screen.Show()
	}
	draw()
	for {
		switch e := screen.PollEvent().(type) {
		case *tcell.EventResize:
			screen.Sync()
			draw()
		case *tcell.EventKey:
			switch e.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC:
				return networkInterface{}, fmt.Errorf("network startup cancelled")
			case tcell.KeyUp:
				selected = (selected + len(candidates) - 1) % len(candidates)
			case tcell.KeyDown:
				selected = (selected + 1) % len(candidates)
			case tcell.KeyEnter:
				return candidates[selected], nil
			}
			draw()
		case nil:
			return networkInterface{}, fmt.Errorf("network startup cancelled")
		}
	}
}
func networkDefaults(name string) (string, string, error) {
	candidates, err := discoverInterfaces()
	if err != nil {
		return "", "", err
	}
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("no usable network interface: need an up, multicast-capable interface with a non-loopback IPv4 address; connect a network or use --offline")
	}
	var selected networkInterface
	if name != "" {
		for _, c := range candidates {
			if c.name == name {
				selected = c
			}
		}
		if selected.name == "" {
			return "", "", fmt.Errorf("interface %q has no usable multicast IPv4 address", name)
		}
	} else if len(candidates) == 1 {
		selected = candidates[0]
	} else {
		selected, err = chooseInterface(candidates)
		if err != nil {
			return "", "", err
		}
	}
	return net.JoinHostPort(selected.addresses[0], "3610"), selected.name, nil
}
