//go:build windows

package win

// Live per-device throughput from the USB stack's ETW bus-trace events.
//
// UCX emits a Start/Stop pair per URB. On Stop with a success status the
// URB's TransferBufferLength is the number of bytes that actually moved,
// and TransferFlags bit 0 gives the direction (1 = IN, i.e. a read from
// the device). fid_UsbDevice is a kernel object pointer; the Rundown
// keyword makes USBHUB3 emit, at session start, the device interface path
// for every such pointer, which converts to the PnP instance ID the
// topology uses. A hotplug event restarts the session so newly arrived
// devices get a mapping too.

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"portauthority/core/model"
	"portauthority/core/provider"
)

const (
	throughputSessionName = "PortAuthority-USB"
	throughputKeywords    = ETWKeywordDefault | ETWKeywordHeadersBusTrace | ETWKeywordRundown
	throughputInterval    = time.Second
)

// throughputAvailable reports whether this process can start an ETW
// session: elevated administrator, or a member of Performance Log Users.
func (p *Provider) throughputAvailable() bool {
	token := windows.GetCurrentProcessToken()
	for _, kind := range []windows.WELL_KNOWN_SID_TYPE{windows.WinBuiltinAdministratorsSid, windows.WinBuiltinPerfLoggingUsersSid} {
		sid, err := windows.CreateWellKnownSid(kind)
		if err != nil {
			continue
		}
		if member, err := token.IsMember(sid); err == nil && member {
			return true
		}
	}
	return false
}

// onHotplug is wired into Watch so the throughput session can refresh its
// device map after topology changes.
func (p *Provider) onHotplug(model.TopologyEvent) {
	p.restartMu.Lock()
	restart := p.restartThroughput
	p.restartMu.Unlock()
	if restart == nil {
		return
	}
	select {
	case restart <- struct{}{}:
	default:
	}
}

type byteCounter struct {
	read, write uint64
}

type throughputMonitor struct {
	mu       sync.Mutex
	paths    map[string]string       // fid_UsbDevice pointer -> canonical instance ID
	counters map[string]*byteCounter // instance ID (or "ptr:...") -> bytes since last tick
	active   map[string]bool         // devices that moved bytes in the previous tick
}

func newThroughputMonitor() *throughputMonitor {
	return &throughputMonitor{
		paths:    map[string]string{},
		counters: map[string]*byteCounter{},
		active:   map[string]bool{},
	}
}

// Throughput streams one sample per second per active device until ctx is
// cancelled. Requires ETW session rights (see throughputAvailable).
func (p *Provider) Throughput(ctx context.Context) (<-chan model.ThroughputSample, error) {
	if !p.throughputAvailable() {
		return nil, provider.ErrUnsupported
	}
	restart := make(chan struct{}, 1)
	p.restartMu.Lock()
	p.restartThroughput = restart
	p.restartMu.Unlock()

	out := make(chan model.ThroughputSample, 256)
	m := newThroughputMonitor()
	go m.run(ctx, restart, out)
	go func() {
		<-ctx.Done()
		p.restartMu.Lock()
		if p.restartThroughput == restart {
			p.restartThroughput = nil
		}
		p.restartMu.Unlock()
	}()
	return out, nil
}

func (m *throughputMonitor) run(ctx context.Context, restart <-chan struct{}, out chan<- model.ThroughputSample) {
	defer close(out)
	providers := []ETWProvider{
		{Name: "USBHUB3", GUID: ETWProviderUSBHUB3, Keywords: throughputKeywords, Level: 5},
		{Name: "USBXHCI", GUID: ETWProviderUSBXHCI, Keywords: throughputKeywords, Level: 5},
		{Name: "UCX", GUID: ETWProviderUCX, Keywords: throughputKeywords, Level: 5},
	}

	ticker := time.NewTicker(throughputInterval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				for _, s := range m.tick(now) {
					select {
					case out <- s:
					default:
					}
				}
			}
		}
	}()

	for ctx.Err() == nil {
		sessionCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			_, err := RunETWSession(sessionCtx, throughputSessionName, providers, m.handle)
			done <- err
		}()
		select {
		case <-ctx.Done():
			cancel()
			<-done
			return
		case <-restart:
			// Give the PnP manager a moment to finish enumerating, then
			// restart so the rundown covers the new device.
			time.Sleep(500 * time.Millisecond)
			cancel()
			<-done
		case err := <-done:
			cancel()
			if err != nil {
				// Session failed (rights revoked, provider missing). Back off
				// rather than spinning, then try again.
				log.Printf("throughput: etw session ended: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}
}

// handle runs on the ETW thread; keep it cheap.
func (m *throughputMonitor) handle(ev *ETWEvent) {
	dev := pointerProperty(ev, "fid_UsbDevice")
	if dev == "" {
		return
	}
	if path, ok := ev.Property("fid_DeviceInterfacePath"); ok {
		if s, ok := path.(string); ok && s != "" {
			m.mu.Lock()
			m.paths[dev] = canonicalInstanceID(instanceIDFromInterface(s))
			m.mu.Unlock()
		}
		return
	}
	if ev.OpcodeName != "Stop" {
		return
	}
	if status, ok := ev.Property("fid_IRP_NtStatus"); ok {
		if v, ok := asUint64(status); !ok || v != 0 {
			return
		}
	}
	length, flags, ok := urbLengthAndFlags(ev)
	if !ok || length == 0 {
		return
	}
	m.mu.Lock()
	id := m.paths[dev]
	if id == "" {
		id = "ptr:" + dev
	}
	c := m.counters[id]
	if c == nil {
		c = &byteCounter{}
		m.counters[id] = c
	}
	if flags&1 != 0 {
		c.read += length
	} else {
		c.write += length
	}
	m.mu.Unlock()
}

// tick converts the bytes counted since the last tick into samples. A
// device that just went idle gets one zero sample so consumers can decay.
func (m *throughputMonitor) tick(now time.Time) []model.ThroughputSample {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.ThroughputSample
	seen := map[string]bool{}
	for id, c := range m.counters {
		out = append(out, model.ThroughputSample{
			DeviceID: id, At: now,
			ReadBps:  model.Bitrate(c.read * 8),
			WriteBps: model.Bitrate(c.write * 8),
		})
		seen[id] = true
		delete(m.counters, id)
	}
	for id := range m.active {
		if !seen[id] {
			out = append(out, model.ThroughputSample{DeviceID: id, At: now})
		}
	}
	m.active = seen
	return out
}

func pointerProperty(ev *ETWEvent, name string) string {
	v, ok := ev.Property(name)
	if !ok {
		return ""
	}
	n, ok := asUint64(v)
	if !ok || n == 0 {
		return ""
	}
	return "0x" + strconvHex64(n)
}

// urbLengthAndFlags digs TransferBufferLength and TransferFlags out of
// whichever per-URB-function struct the event carries.
func urbLengthAndFlags(ev *ETWEvent) (length, flags uint64, ok bool) {
	for _, p := range ev.Properties {
		members, isStruct := p.Value.([]ETWProperty)
		if !isStruct || !strings.HasPrefix(p.Name, "fid_UCX_URB_") {
			continue
		}
		var haveLen bool
		for _, mem := range members {
			switch mem.Name {
			case "fid_URB_TransferBufferLength":
				length, haveLen = asUint64(mem.Value)
			case "fid_URB_TransferFlags":
				flags, _ = asUint64(mem.Value)
			}
		}
		if haveLen {
			return length, flags, true
		}
	}
	return 0, 0, false
}

func asUint64(v any) (uint64, bool) {
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

func strconvHex64(v uint64) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v&0xf]
		v >>= 4
	}
	return string(b[i:])
}
