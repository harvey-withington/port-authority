// pactl is the Port Authority collector CLI: topology snapshots, a text
// tree, plain-language insights, the local HTTP API, and the ETW spike.
// Run it without arguments for usage. --fixture replays a saved snapshot
// through the mock provider instead of touching hardware.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"portauthority/core/enrich"
	"portauthority/core/kb"
	"portauthority/core/model"
	"portauthority/core/provider"
	"portauthority/core/provider/mock"
	"portauthority/platform"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	// Every command sees the user's own docks; serve can point elsewhere.
	if dir, derr := kb.DefaultLocalDir(); derr == nil {
		if lerr := kb.UseLocalDir(dir); lerr != nil {
			fmt.Fprintf(os.Stderr, "pactl: local knowledge base in %s not loaded: %v\n", dir, lerr)
		}
	}
	switch cmd {
	case "snapshot":
		err = runSnapshot(args)
	case "tree":
		err = runTree(args)
	case "caps":
		err = runCaps(args)
	case "etw-spike":
		err = runETWSpike(args)
	case "insights":
		err = runInsights(args)
	case "serve":
		err = runServe(args)
	case "live":
		err = runLive(args)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pactl:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  pactl snapshot [--pretty] [--fixture file]   print the topology as JSON
  pactl tree     [--all]    [--fixture file]   print the topology as a tree
  pactl caps                                   print provider capabilities
  pactl insights [--json] [--fixture file]     plain-language findings about what is connected
  pactl serve    [--addr 127.0.0.1:7911] [--fixture file]   host the local HTTP API until Ctrl+C
  pactl live     [--seconds N] [--quiet]       print hotplug events and live throughput per device
  pactl etw-spike [--seconds N]                (admin) capture USB ETW bus-trace events and report`)
}

func selectProvider(fixture string) (provider.Provider, error) {
	if fixture != "" {
		return mock.Load(fixture)
	}
	return platform.New()
}

func snapshot(fixture string) (*model.Topology, error) {
	p, err := selectProvider(fixture)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t, err := p.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	enrich.Annotate(t)
	return t, nil
}

func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	pretty := fs.Bool("pretty", false, "indent the JSON")
	fixture := fs.String("fixture", "", "replay a saved snapshot instead of reading hardware")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := snapshot(*fixture)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	if *pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(t)
}

func runCaps(args []string) error {
	p, err := platform.New()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(p.Capabilities())
}

func runTree(args []string) error {
	fs := flag.NewFlagSet("tree", flag.ExitOnError)
	all := fs.Bool("all", false, "include empty ports")
	fixture := fs.String("fixture", "", "replay a saved snapshot instead of reading hardware")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := snapshot(*fixture)
	if err != nil {
		return err
	}
	for i := range t.Controllers {
		c := &t.Controllers[i]
		fmt.Printf("%s  [%s, %s]\n", c.Name, c.Kind, c.MaxBandwidth)
		if c.RootHub != nil {
			printHub(c.RootHub, "  ", *all)
		}
	}
	if len(t.USB4) > 0 {
		fmt.Println("USB4 / Thunderbolt")
		printUSB4(t.USB4)
	}
	for _, w := range t.Warnings {
		fmt.Printf("warning: %s\n", w)
	}
	return nil
}

func printHub(dev *model.Device, indent string, all bool) {
	hub := dev.Hub
	if hub == nil {
		return
	}
	for _, p := range hub.Ports {
		if p.Device == nil && !all {
			continue
		}
		conn := ""
		if p.Connector != nil && p.Connector.TypeC {
			conn = " type-c"
		}
		if p.Device == nil {
			fmt.Printf("%sport %d: %s  max=%s%s\n", indent, p.Number, p.Status, p.MaxLink, conn)
			continue
		}
		d := p.Device
		name := d.Product
		if name == "" {
			name = d.FriendlyName
		}
		if name == "" {
			name = d.Description
		}
		if name == "" {
			name = d.ProductName
		}
		if name == "" {
			name = "(unnamed)"
		}
		vendor := d.VendorName
		if vendor == "" {
			vendor = d.Manufacturer
		}
		if vendor != "" {
			name = vendor + " " + name
		}
		fmt.Printf("%sport %d: %s [%04x:%04x] %s  link=%s claimed=%s port-max=%s%s",
			indent, p.Number, name, d.VendorID, d.ProductID, d.Class,
			p.NegotiatedLink, d.ClaimedSpeed, p.MaxLink, conn)
		if d.PowerDrawMA > 0 {
			fmt.Printf(" %dmA", d.PowerDrawMA)
		}
		if d.IsoReserved > 0 {
			fmt.Printf(" iso=%s", d.IsoReserved)
		}
		if p.NegotiatedLink < d.ClaimedSpeed && p.NegotiatedLink < p.MaxLink {
			fmt.Print("  <-- DOWNGRADED")
		}
		fmt.Println()
		if d.Hub != nil {
			fmt.Printf("%s  hub: %s, %d ports, depth %d\n", indent, d.Hub.Kind, d.Hub.PortCount, d.Hub.Depth)
			printHub(d, indent+"    ", all)
		}
	}
}

// printUSB4 lists routers indented by depth, with the non-USB devices
// (NVMe disks and the like) each one carries.
func printUSB4(routers []model.USB4Router) {
	for _, r := range routers {
		indent := strings.Repeat("    ", r.Depth+1)
		name := r.Name
		if name == "" {
			name = r.InstanceID
		}
		fmt.Printf("%s%s [%04x:%04x] %s router  %s\n", indent, name, r.VendorID, r.ProductID, r.Kind, r.InstanceID)
		for _, c := range r.Children {
			fmt.Printf("%s    carries: %s\n", indent, c)
		}
	}
}
