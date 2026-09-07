package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"portauthority/core/insight"
	"portauthority/core/model"
	"portauthority/core/provider"
	"portauthority/core/provider/mock"
)

// fakeProvider serves a swappable topology and hands Run channels the
// test controls. A nil events or samples channel makes the corresponding
// stream report ErrUnsupported.
type fakeProvider struct {
	mu      sync.Mutex
	topo    *model.Topology
	events  chan model.TopologyEvent
	samples chan model.ThroughputSample
	// watchErr / throughputErr override ErrUnsupported when set.
	watchErr, throughputErr error
	snapshots               int
}

func newFakeProvider(t *testing.T, topo *model.Topology) *fakeProvider {
	t.Helper()
	return &fakeProvider{
		topo:    topo,
		events:  make(chan model.TopologyEvent, 16),
		samples: make(chan model.ThroughputSample, 16),
	}
}

func (f *fakeProvider) set(topo *model.Topology) {
	f.mu.Lock()
	f.topo = topo
	f.mu.Unlock()
}

func (f *fakeProvider) Snapshot(ctx context.Context) (*model.Topology, error) {
	f.mu.Lock()
	topo := f.topo
	f.snapshots++
	f.mu.Unlock()
	return mock.New(topo).Snapshot(ctx)
}

// Watch honours the provider stream contract: the returned channel closes
// once ctx is cancelled. Tests keep writing to f.events, which is never
// closed, so a late write cannot panic.
func (f *fakeProvider) Watch(ctx context.Context) (<-chan model.TopologyEvent, error) {
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	if f.events == nil {
		return nil, provider.ErrUnsupported
	}
	out := make(chan model.TopologyEvent, 16)
	go forwardUntilDone(ctx, f.events, out)
	return out, nil
}

func (f *fakeProvider) Throughput(ctx context.Context) (<-chan model.ThroughputSample, error) {
	if f.throughputErr != nil {
		return nil, f.throughputErr
	}
	if f.samples == nil {
		return nil, provider.ErrUnsupported
	}
	out := make(chan model.ThroughputSample, 16)
	go forwardUntilDone(ctx, f.samples, out)
	return out, nil
}

// forwardUntilDone copies in to out and closes out when ctx ends.
func forwardUntilDone[T any](ctx context.Context, in <-chan T, out chan<- T) {
	defer close(out)
	for {
		select {
		case <-ctx.Done():
			return
		case v, ok := <-in:
			if !ok {
				return
			}
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (f *fakeProvider) Capabilities() model.ProviderCaps {
	return model.ProviderCaps{Platform: "fake", Topology: true, Hotplug: f.events != nil, Throughput: f.samples != nil}
}

// fixtureVariants loads the CalDigit TS4 fixture and returns it as is
// (with the EX400U on hub 2188:5501 port 3) and a copy with that port
// emptied.
func fixtureVariants(t *testing.T) (withSSD, withoutSSD *model.Topology) {
	t.Helper()
	load := func() *model.Topology {
		p, err := mock.Load(fixturePath)
		if err != nil {
			t.Fatalf("load fixture: %v", err)
		}
		topo, err := p.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return topo
	}
	withSSD = load()
	withoutSSD = load()
	removed := false
	withoutSSD.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		if d.VendorID != 0x2188 || d.ProductID != 0x5501 || d.Hub == nil {
			return
		}
		for i := range d.Hub.Ports {
			p := &d.Hub.Ports[i]
			if p.Number == 3 && p.Device != nil && p.Device.ID == ex400uID {
				p.Device = nil
				p.Status = model.StatusNoDevice
				p.NegotiatedLink = model.LinkNone
				removed = true
			}
		}
	})
	if !removed {
		t.Fatal("fixture has no EX400U on hub 2188:5501 port 3")
	}
	if findDevice(withoutSSD, ex400uID) != nil {
		t.Fatal("EX400U still present after removal")
	}
	return withSSD, withoutSSD
}

// startRun launches svc.Run and fails the test if it returns an error.
// The returned function stops it and waits for it to exit.
func startRun(t *testing.T, svc *Service) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	return func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run did not return after cancel")
		}
	}
}

// waitEvent receives the next event of the given type from ch, failing
// after timeout. Other types received meanwhile are returned in skipped.
func waitEvent(t *testing.T, ch <-chan Event, typ string, timeout time.Duration) (ev Event, skipped []Event) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case e := <-ch:
			if e.Type == typ {
				return e, skipped
			}
			skipped = append(skipped, e)
		case <-deadline:
			t.Fatalf("no %s event within %v (saw %d other events)", typ, timeout, len(skipped))
		}
	}
}

// expectNoEvent asserts that nothing arrives on ch for d.
func expectNoEvent(t *testing.T, ch <-chan Event, d time.Duration) {
	t.Helper()
	select {
	case e := <-ch:
		t.Fatalf("unexpected %s event", e.Type)
	case <-time.After(d):
	}
}

func TestRunDebouncesHotplugIntoOneTopologyChanged(t *testing.T) {
	withSSD, _ := fixtureVariants(t)
	fp := newFakeProvider(t, withSSD)
	svc := NewService(fp)
	events, unsub := svc.Subscribe()
	defer unsub()
	stop := startRun(t, svc)
	defer stop()

	// Wait for the baseline snapshot so counts are deterministic.
	waitFor(t, func() bool { fp.mu.Lock(); defer fp.mu.Unlock(); return fp.snapshots >= 1 })
	start := time.Now()
	for i := 0; i < 3; i++ {
		fp.events <- model.TopologyEvent{Kind: model.EventDeviceAdded, At: time.Now(), DeviceID: "dev", Detail: "burst"}
		time.Sleep(50 * time.Millisecond)
	}
	// Nothing fires while events keep arriving inside the debounce window.
	expectNoEvent(t, events, 100*time.Millisecond)

	ev, _ := waitEvent(t, events, EventTopologyChanged, 3*time.Second)
	if elapsed := time.Since(start); elapsed < svc.debounce {
		t.Errorf("topology_changed after %v, before the %v debounce", elapsed, svc.debounce)
	}
	tc, ok := ev.Data.(TopologyChanged)
	if !ok {
		t.Fatalf("data is %T, want TopologyChanged", ev.Data)
	}
	if len(tc.Events) != 3 {
		t.Errorf("topology_changed carried %d events, want 3", len(tc.Events))
	}
	if tc.Controllers != len(withSSD.Controllers) || tc.Devices == 0 || tc.CapturedAt.IsZero() || ev.At.IsZero() {
		t.Errorf("topology_changed = %+v", tc)
	}
	// One burst, one fresh snapshot (plus the baseline).
	fp.mu.Lock()
	n := fp.snapshots
	fp.mu.Unlock()
	if n != 2 {
		t.Errorf("provider snapshots = %d, want 2 (baseline + one refresh)", n)
	}
	expectNoEvent(t, events, 200*time.Millisecond)
}

func TestRunEmitsInsightResolvedAndAdded(t *testing.T) {
	withSSD, withoutSSD := fixtureVariants(t)
	fp := newFakeProvider(t, withSSD)
	svc := NewService(fp)
	events, unsub := svc.Subscribe(EventInsightAdded, EventInsightResolved)
	defer unsub()
	stop := startRun(t, svc)
	defer stop()
	waitFor(t, func() bool { fp.mu.Lock(); defer fp.mu.Unlock(); return fp.snapshots >= 1 })

	// Unplug the EX400U: its faster-port-available insight goes away.
	fp.set(withoutSSD)
	fp.events <- model.TopologyEvent{Kind: model.EventDeviceRemoved, At: time.Now(), DeviceID: ex400uID}
	ev, skipped := waitEvent(t, events, EventInsightResolved, 3*time.Second)
	for _, s := range skipped {
		t.Errorf("unexpected %s before insight_resolved", s.Type)
	}
	in, ok := ev.Data.(insight.Insight)
	if !ok || in.RuleID != ex400uRule || !contains(in.DeviceIDs, ex400uID) {
		t.Errorf("insight_resolved = %+v, want %s for the EX400U", ev.Data, ex400uRule)
	}

	// Plug it back in: the insight is added again.
	fp.set(withSSD)
	fp.events <- model.TopologyEvent{Kind: model.EventDeviceAdded, At: time.Now(), DeviceID: ex400uID}
	ev, skipped = waitEvent(t, events, EventInsightAdded, 3*time.Second)
	for _, s := range skipped {
		t.Errorf("unexpected %s before insight_added", s.Type)
	}
	in, ok = ev.Data.(insight.Insight)
	if !ok || in.RuleID != ex400uRule || !contains(in.DeviceIDs, ex400uID) {
		t.Errorf("insight_added = %+v, want %s for the EX400U", ev.Data, ex400uRule)
	}

	// The HTTP snapshot was invalidated: it reflects the current topology.
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()
	var got struct {
		Insights []insight.Insight `json:"insights"`
	}
	get(t, srv, "/api/v1/insights", http.StatusOK, &got)
	if !hasRule(got.Insights, ex400uRule) {
		t.Errorf("insights after re-plug lack %s", ex400uRule)
	}
}

func TestRunForwardsThroughputSamples(t *testing.T) {
	withSSD, _ := fixtureVariants(t)
	fp := newFakeProvider(t, withSSD)
	svc := NewService(fp)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	var empty struct {
		SchemaVersion int                      `json:"schema_version"`
		Samples       []model.ThroughputSample `json:"samples"`
	}
	get(t, srv, "/api/v1/throughput", http.StatusOK, &empty)
	if empty.SchemaVersion != 1 || empty.Samples == nil || len(empty.Samples) != 0 {
		t.Errorf("throughput before Run = %+v, want empty samples array", empty)
	}

	events, unsub := svc.Subscribe(EventThroughputSample)
	defer unsub()
	stop := startRun(t, svc)
	defer stop()

	first := model.ThroughputSample{DeviceID: ex400uID, At: time.Now(), ReadBps: 100 * model.Mbps, WriteBps: 10 * model.Mbps}
	second := model.ThroughputSample{DeviceID: ex400uID, At: time.Now(), ReadBps: 200 * model.Mbps, WriteBps: 20 * model.Mbps}
	other := model.ThroughputSample{DeviceID: "dev-other", At: time.Now(), ReadBps: 1 * model.Mbps}
	for _, smp := range []model.ThroughputSample{first, second, other} {
		fp.samples <- smp
		ev, _ := waitEvent(t, events, EventThroughputSample, 2*time.Second)
		got, ok := ev.Data.(model.ThroughputSample)
		if !ok || got.DeviceID != smp.DeviceID || got.ReadBps != smp.ReadBps {
			t.Errorf("throughput_sample = %+v, want %+v", ev.Data, smp)
		}
	}

	var got struct {
		Samples []model.ThroughputSample `json:"samples"`
	}
	get(t, srv, "/api/v1/throughput", http.StatusOK, &got)
	if len(got.Samples) != 2 {
		t.Fatalf("throughput samples = %+v, want latest per device (2)", got.Samples)
	}
	// Ordered by device id: "USB\..." sorts before "dev-other".
	if got.Samples[0].DeviceID != ex400uID || got.Samples[0].ReadBps != second.ReadBps {
		t.Errorf("sample[0] = %+v, want latest EX400U sample", got.Samples[0])
	}
	if got.Samples[1].DeviceID != other.DeviceID {
		t.Errorf("sample[1] = %+v, want dev-other", got.Samples[1])
	}
}

func TestRecentSamplesExpire(t *testing.T) {
	svc := NewService(newFakeProvider(t, nil))
	svc.samplesMu.Lock()
	svc.samples["old"] = storedSample{sample: model.ThroughputSample{DeviceID: "old"}, seenAt: time.Now().Add(-sampleMaxAge - time.Second)}
	svc.samples["new"] = storedSample{sample: model.ThroughputSample{DeviceID: "new"}, seenAt: time.Now()}
	svc.samplesMu.Unlock()
	got := svc.RecentSamples()
	if len(got) != 1 || got[0].DeviceID != "new" {
		t.Errorf("RecentSamples = %+v, want only the fresh one", got)
	}
}

func TestRunWithUnsupportedStreams(t *testing.T) {
	withSSD, _ := fixtureVariants(t)
	fp := newFakeProvider(t, withSSD)
	fp.events, fp.samples = nil, nil
	svc := NewService(fp)
	stop := startRun(t, svc)
	// Run stays up (nothing to consume) and the HTTP side still works.
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()
	get(t, srv, "/api/v1/topology", http.StatusOK, nil)
	get(t, srv, "/api/v1/throughput", http.StatusOK, nil)
	stop()

	// The mock provider is the real-world case: Watch works, Throughput
	// is unsupported.
	mp, err := mock.Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	stop = startRun(t, NewService(mp))
	stop()

	// A genuine Watch failure is reported.
	bad := newFakeProvider(t, withSSD)
	bad.watchErr = errors.New("etw session refused")
	if err := NewService(bad).Run(context.Background()); err == nil || !strings.Contains(err.Error(), "etw session refused") {
		t.Errorf("Run with failing Watch = %v, want the watch error", err)
	}
}

func TestHubDropsForSlowSubscriber(t *testing.T) {
	h := newHub()
	slow := h.subscribe(nil)
	filtered := h.subscribe([]string{EventInsightAdded})
	for i := 0; i < subscriberBuffer+10; i++ {
		h.broadcast(Event{Type: EventThroughputSample})
	}
	if got := slow.dropped.Load(); got != 10 {
		t.Errorf("dropped = %d, want 10", got)
	}
	if len(slow.ch) != subscriberBuffer {
		t.Errorf("buffered = %d, want %d", len(slow.ch), subscriberBuffer)
	}
	if len(filtered.ch) != 0 || filtered.dropped.Load() != 0 {
		t.Errorf("filtered subscriber received throughput events")
	}
	h.unsubscribe(slow)
	h.unsubscribe(slow) // idempotent
	if h.count() != 1 {
		t.Errorf("count = %d after unsubscribe, want 1", h.count())
	}
}

func TestParseTypes(t *testing.T) {
	if got := parseTypes(""); got != nil {
		t.Errorf("parseTypes(\"\") = %v, want nil", got)
	}
	got := parseTypes(" throughput_sample, topology_changed ,bogus,,topology_changed")
	want := []string{EventThroughputSample, EventTopologyChanged}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("parseTypes = %v, want %v", got, want)
	}
	if got := parseTypes("bogus"); got != nil {
		t.Errorf("parseTypes(bogus) = %v, want nil (all)", got)
	}
}

// wsMessage is the wire shape of a stream event with the data left raw.
type wsMessage struct {
	Type string          `json:"type"`
	At   time.Time       `json:"at"`
	Data json.RawMessage `json:"data"`
}

func dialStream(t *testing.T, srv *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/stream" + query
	c, resp, err := websocket.Dial(ctx, u, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial %s: %v (status %d)", u, err, status)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func readMessage(t *testing.T, c *websocket.Conn) wsMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	typ, body, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if typ != websocket.MessageText {
		t.Errorf("message type = %v, want text", typ)
	}
	var m wsMessage
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if m.At.IsZero() {
		t.Errorf("message %s has no at", m.Type)
	}
	return m
}

func TestStreamHelloThenEvents(t *testing.T) {
	withSSD, withoutSSD := fixtureVariants(t)
	fp := newFakeProvider(t, withSSD)
	svc := NewService(fp)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()
	stop := startRun(t, svc)
	defer stop()

	c := dialStream(t, srv, "")
	hello := readMessage(t, c)
	if hello.Type != EventHello {
		t.Fatalf("first message = %s, want hello", hello.Type)
	}
	var h Hello
	if err := json.Unmarshal(hello.Data, &h); err != nil {
		t.Fatal(err)
	}
	if h.SchemaVersion != 1 || h.Capabilities.Platform != "fake" || !h.Capabilities.Hotplug {
		t.Errorf("hello = %+v", h)
	}
	if !hasRule(h.Insights, ex400uRule) || h.CapturedAt.IsZero() {
		t.Errorf("hello insights lack %s: %+v", ex400uRule, h)
	}
	waitFor(t, func() bool { return svc.hub.count() == 1 })

	// Whatever the client sends is discarded; the stream keeps flowing.
	wctx, wcancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := c.Write(wctx, websocket.MessageText, []byte(`{"ignored": true}`)); err != nil {
		t.Errorf("client write: %v", err)
	}
	wcancel()

	fp.set(withoutSSD)
	fp.events <- model.TopologyEvent{Kind: model.EventDeviceRemoved, At: time.Now(), DeviceID: ex400uID}
	var seen []string
	for i := 0; i < 2; i++ {
		seen = append(seen, readMessage(t, c).Type)
	}
	if seen[0] != EventTopologyChanged || seen[1] != EventInsightResolved {
		t.Errorf("events after unplug = %v, want [topology_changed insight_resolved]", seen)
	}

	fp.samples <- model.ThroughputSample{DeviceID: "dev", At: time.Now(), ReadBps: model.Mbps}
	if m := readMessage(t, c); m.Type != EventThroughputSample {
		t.Errorf("got %s, want throughput_sample", m.Type)
	}

	// A clean client close releases the subscription.
	if err := c.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Errorf("close: %v", err)
	}
	waitFor(t, func() bool { return svc.hub.count() == 0 })
}

func TestStreamTypesFilter(t *testing.T) {
	withSSD, _ := fixtureVariants(t)
	fp := newFakeProvider(t, withSSD)
	svc := NewService(fp)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()
	stop := startRun(t, svc)
	defer stop()

	c := dialStream(t, srv, "?types=throughput_sample,bogus")
	hello := readMessage(t, c)
	var h Hello
	if err := json.Unmarshal(hello.Data, &h); err != nil {
		t.Fatal(err)
	}
	if len(h.Types) != 1 || h.Types[0] != EventThroughputSample {
		t.Errorf("hello types = %v, want [throughput_sample]", h.Types)
	}
	waitFor(t, func() bool { return svc.hub.count() == 1 })

	// A hotplug refresh is filtered out; a sample gets through.
	fp.events <- model.TopologyEvent{Kind: model.EventResnapshot, At: time.Now()}
	time.Sleep(svc.debounce + 200*time.Millisecond)
	fp.samples <- model.ThroughputSample{DeviceID: "dev", At: time.Now(), ReadBps: model.Mbps}
	if m := readMessage(t, c); m.Type != EventThroughputSample {
		t.Errorf("filtered stream delivered %s, want throughput_sample", m.Type)
	}
}

func TestStreamWithoutRun(t *testing.T) {
	// The endpoint works with no Run: hello arrives and the connection
	// stays open until the client leaves.
	srv := newFixtureServer(t)
	c := dialStream(t, srv, "")
	if m := readMessage(t, c); m.Type != EventHello {
		t.Errorf("first message = %s, want hello", m.Type)
	}

	// Pings are answered by the server's discard loop. The client needs a
	// concurrent reader of its own to see the pong.
	readErr := make(chan error, 1)
	go func() {
		for {
			if _, _, err := c.Read(context.Background()); err != nil {
				readErr <- err
				return
			}
		}
	}()
	pctx, pcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pcancel()
	if err := c.Ping(pctx); err != nil {
		t.Errorf("ping: %v", err)
	}
	if err := c.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Errorf("close: %v", err)
	}
	select {
	case err := <-readErr:
		if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			t.Errorf("reader ended with %v, want normal closure", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("client reader did not end after close")
	}
}

func TestStreamClosesOnServerContext(t *testing.T) {
	withSSD, _ := fixtureVariants(t)
	svc := NewService(newFakeProvider(t, withSSD))
	ctx, cancel := context.WithCancel(context.Background())
	addr := freeAddr(t)
	done := make(chan error, 1)
	go func() { done <- ListenAndServe(ctx, addr, svc.Handler()) }()
	waitFor(t, func() bool {
		resp, err := http.Get("http://" + addr + "/api/v1/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	})

	dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dcancel()
	c, _, err := websocket.Dial(dctx, "ws://"+addr+"/api/v1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if _, _, err := c.Read(dctx); err != nil {
		t.Fatalf("hello: %v", err)
	}

	cancel()
	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	if _, _, err := c.Read(rctx); err == nil {
		t.Error("stream still open after server context cancelled")
	} else if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Errorf("close status = %v (%v), want normal closure", websocket.CloseStatus(err), err)
	}
	if err := <-done; err != nil {
		t.Errorf("ListenAndServe = %v", err)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within 5s")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
