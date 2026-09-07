package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"portauthority/core/model"
	"portauthority/core/provider"
	"portauthority/platform"
)

// runLive prints hotplug events and per-device throughput as they happen.
// It is the terminal view of what the WebSocket stream carries.
func runLive(args []string) error {
	fs := flag.NewFlagSet("live", flag.ExitOnError)
	seconds := fs.Int("seconds", 0, "stop after this many seconds (0 = until Ctrl+C)")
	quiet := fs.Bool("quiet", false, "hide idle (zero) throughput samples")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := platform.New()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *seconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*seconds)*time.Second)
		defer cancel()
	}

	caps := p.Capabilities()
	fmt.Fprintf(os.Stderr, "hotplug=%v throughput=%v  (Ctrl+C to stop)\n", caps.Hotplug, caps.Throughput)

	events, err := p.Watch(ctx)
	if err != nil && !errors.Is(err, provider.ErrUnsupported) {
		return fmt.Errorf("watch: %w", err)
	}
	samples, err := p.Throughput(ctx)
	if err != nil && !errors.Is(err, provider.ErrUnsupported) {
		return fmt.Errorf("throughput: %w", err)
	}
	if samples == nil {
		fmt.Fprintln(os.Stderr, "throughput unavailable: needs administrator or Performance Log Users membership")
	}

	names := deviceNames()
	for events != nil || samples != nil {
		select {
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			fmt.Printf("%s  %-14s %s  %s\n", ev.At.Format("15:04:05.000"), ev.Kind, nameFor(names, ev.DeviceID), ev.DeviceID)
			if ev.Kind == model.EventDeviceAdded {
				names = deviceNames() // pick up the new device's name
			}
		case s, ok := <-samples:
			if !ok {
				samples = nil
				continue
			}
			if *quiet && s.ReadBps == 0 && s.WriteBps == 0 {
				continue
			}
			fmt.Printf("%s  %-14s %-32s read %10s  write %10s\n", s.At.Format("15:04:05.000"), "throughput",
				nameFor(names, s.DeviceID), bytesPerSecond(s.ReadBps), bytesPerSecond(s.WriteBps))
		}
	}
	return nil
}

func bytesPerSecond(b model.Bitrate) string {
	mb := float64(b) / 8 / 1e6
	switch {
	case b == 0:
		return "idle"
	case mb >= 1:
		return fmt.Sprintf("%.1f MB/s", mb)
	default:
		return fmt.Sprintf("%.0f KB/s", mb*1000)
	}
}

// deviceNames snapshots the topology once to label events by name.
func deviceNames() map[string]string {
	names := map[string]string{}
	t, err := snapshot("")
	if err != nil {
		return names
	}
	t.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		name := d.Product
		if name == "" {
			name = d.ProductName
		}
		if name == "" {
			name = d.FriendlyName
		}
		if name == "" {
			name = d.Description
		}
		if d.VendorName != "" && name != "" {
			name = d.VendorName + " " + name
		}
		names[canonical(d.ID)] = name
	})
	return names
}

func nameFor(names map[string]string, id string) string {
	if n := names[canonical(id)]; n != "" {
		return n
	}
	return "(unknown device)"
}

func canonical(id string) string {
	b := []byte(id)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}
