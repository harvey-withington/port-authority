//go:build windows

package main

// Phase 0 spike: can we consume the USB stack's ETW bus-trace events from
// Go, and do they carry enough to attribute bytes to a device?
//
// Usage:
//
//	pactl etw-spike --seconds 20 [--samples 1] [--keywords 0x8041] [--verbose]
//
// While it runs, copy a large file to or from a USB drive. The report
// lists every event type seen with decoded sample payloads, and sums
// completed transfer lengths per device, resolved to the device's PnP
// interface path via the rundown events emitted when the session starts.
//
// Needs administrator rights, or membership of the Performance Log Users
// group.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"portauthority/platform/win"
)

type spikeKey struct {
	provider string
	id       uint16
}

type spikeAgg struct {
	count     uint64
	task      string
	opcode    string
	samples   []*win.ETWEvent
	decodeErr string
}

type deviceTotals struct {
	bytes  uint64
	events uint64
}

func runETWSpike(args []string) error {
	fs := flag.NewFlagSet("etw-spike", flag.ExitOnError)
	seconds := fs.Int("seconds", 15, "how long to capture")
	samples := fs.Int("samples", 1, "decoded sample events to keep per event type")
	keywordsHex := fs.String("keywords", "0x8041", "provider keyword mask (hex). 0x8041 = Default|HeadersBusTrace|Rundown")
	level := fs.Int("level", 5, "trace level (5 = verbose)")
	verbose := fs.Bool("verbose", false, "print every event as it arrives")
	if err := fs.Parse(args); err != nil {
		return err
	}
	keywords, err := strconv.ParseUint(strings.TrimPrefix(*keywordsHex, "0x"), 16, 64)
	if err != nil {
		return fmt.Errorf("bad --keywords: %w", err)
	}

	providers := []win.ETWProvider{
		{Name: "USBHUB3", GUID: win.ETWProviderUSBHUB3, Keywords: keywords, Level: uint8(*level)},
		{Name: "USBXHCI", GUID: win.ETWProviderUSBXHCI, Keywords: keywords, Level: uint8(*level)},
		{Name: "UCX", GUID: win.ETWProviderUCX, Keywords: keywords, Level: uint8(*level)},
	}

	aggs := map[spikeKey]*spikeAgg{}
	totals := map[string]*deviceTotals{} // keyed by fid_UsbDevice pointer
	devicePaths := map[string]string{}   // fid_UsbDevice pointer -> interface path (from rundown)
	var total uint64

	handle := func(ev *win.ETWEvent) {
		total++
		k := spikeKey{ev.Provider, ev.ID}
		a := aggs[k]
		if a == nil {
			a = &spikeAgg{task: ev.TaskName, opcode: ev.OpcodeName}
			aggs[k] = a
		}
		a.count++
		if ev.DecodeErr != "" && a.decodeErr == "" {
			a.decodeErr = ev.DecodeErr
		}
		if len(a.samples) < *samples {
			a.samples = append(a.samples, ev)
		}

		dev := stringProperty(ev, "fid_UsbDevice")
		if path := stringProperty(ev, "fid_DeviceInterfacePath"); path != "" && dev != "" {
			devicePaths[dev] = path
		}
		// Count completed transfers only; Start events describe intent.
		if dev != "" && ev.OpcodeName != "Start" {
			if length, ok := transferLength(ev); ok {
				t := totals[dev]
				if t == nil {
					t = &deviceTotals{}
					totals[dev] = t
				}
				t.bytes += length
				t.events++
			}
		}
		if *verbose {
			fmt.Println(formatEvent(ev))
		}
	}

	fmt.Fprintf(os.Stderr, "capturing USB ETW for %ds with keywords 0x%x level %d... copy a big file to a USB drive now\n", *seconds, keywords, *level)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*seconds)*time.Second)
	defer cancel()
	stats, err := win.RunETWSession(ctx, "PortAuthority-USB-Spike", providers, handle)
	if err != nil {
		return err
	}

	fmt.Printf("\n=== ETW spike report ===\n")
	fmt.Printf("duration %s, events %d, lost %d, real-time buffers lost %d\n\n",
		stats.Duration.Round(time.Millisecond), stats.Events, stats.EventsLost, stats.RealTimeBuffersLost)

	keys := make([]spikeKey, 0, len(aggs))
	for k := range aggs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return aggs[keys[i]].count > aggs[keys[j]].count })
	fmt.Printf("--- event types (%d) ---\n", len(keys))
	for _, k := range keys {
		a := aggs[k]
		fmt.Printf("%-8s id=%-4d x%-8d task=%s opcode=%s", k.provider, k.id, a.count, a.task, a.opcode)
		if a.decodeErr != "" {
			fmt.Printf("  decode-error=%s", a.decodeErr)
		}
		fmt.Println()
		for _, s := range a.samples {
			fmt.Printf("    %s\n", formatProperties(s.Properties))
		}
	}

	if len(totals) == 0 {
		fmt.Println("\nno completed transfers with a length were seen")
		return nil
	}
	fmt.Printf("\n--- completed transfer bytes per device ---\n")
	devs := make([]string, 0, len(totals))
	for d := range totals {
		devs = append(devs, d)
	}
	sort.Slice(devs, func(i, j int) bool { return totals[devs[i]].bytes > totals[devs[j]].bytes })
	secs := stats.Duration.Seconds()
	for _, d := range devs {
		t := totals[d]
		path := devicePaths[d]
		if path == "" {
			path = "(no rundown mapping; enable keyword 0x8000)"
		}
		fmt.Printf("%-18s %13d bytes %8.1f MB/s %7d transfers  %s\n", d, t.bytes, float64(t.bytes)/secs/1e6, t.events, path)
	}
	return nil
}

// transferLength finds the transfer length of a URB event. UCX nests it in
// a per-URB-function struct; fall back to any top-level length-like field.
func transferLength(ev *win.ETWEvent) (uint64, bool) {
	for _, p := range ev.Properties {
		members, ok := p.Value.([]win.ETWProperty)
		if !ok {
			continue
		}
		if v, ok := findLength(members); ok {
			return v, true
		}
	}
	return findLength(ev.Properties)
}

// findLength prefers the URB's TransferBufferLength (actual bytes moved on
// completion) over any other length-like field such as the URB header
// length.
func findLength(props []win.ETWProperty) (uint64, bool) {
	for _, want := range []string{"transferbufferlength", "length"} {
		for _, p := range props {
			if !strings.Contains(strings.ToLower(p.Name), want) {
				continue
			}
			if v, ok := asUint(p.Value); ok {
				return v, true
			}
		}
	}
	return 0, false
}

func stringProperty(ev *win.ETWEvent, name string) string {
	v, ok := ev.Property(name)
	if !ok {
		return ""
	}
	return formatValue(v)
}

func asUint(v any) (uint64, bool) {
	switch x := v.(type) {
	case uint64:
		return x, true
	case int64:
		if x >= 0 {
			return uint64(x), true
		}
	}
	return 0, false
}

func formatValue(v any) string {
	switch x := v.(type) {
	case uint64:
		if x > 0xFFFF {
			return fmt.Sprintf("0x%x", x)
		}
		return strconv.FormatUint(x, 10)
	case []win.ETWProperty:
		return "{" + formatProperties(x) + "}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatProperties(props []win.ETWProperty) string {
	parts := make([]string, 0, len(props))
	for _, p := range props {
		parts = append(parts, p.Name+"="+formatValue(p.Value))
	}
	return strings.Join(parts, " ")
}

func formatEvent(ev *win.ETWEvent) string {
	return fmt.Sprintf("%s %s/%d %s.%s %s", ev.At.Format("15:04:05.000"), ev.Provider, ev.ID, ev.TaskName, ev.OpcodeName, formatProperties(ev.Properties))
}
