package api

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"portauthority/core/insight"
	"portauthority/core/model"
	"portauthority/core/provider"
)

const (
	// defaultDebounce is how long Run waits after a hotplug event before
	// re-snapshotting; a single plug often produces several events.
	defaultDebounce = 300 * time.Millisecond
	// maxDebounce caps how long a continuous storm of events can postpone
	// the refresh.
	maxDebounce = 2 * time.Second
	// sampleMaxAge is how long a throughput sample is reported by
	// /api/v1/throughput after it was received.
	sampleMaxAge = 5 * time.Second
)

// Run drives the live side of the service: it consumes the provider's
// hotplug and throughput streams, refreshes the snapshot after hotplug
// bursts and broadcasts events to stream subscribers. A provider that
// does not support a stream simply leaves that feature off. Run blocks
// until ctx is done and returns nil then; it returns an error only when a
// stream could not be started for a reason other than being unsupported.
// The Service works without Run, just without live events.
func (s *Service) Run(ctx context.Context) error {
	events, err := s.p.Watch(ctx)
	switch {
	case errors.Is(err, provider.ErrUnsupported):
		s.logger.Printf("hotplug events not supported by provider %s; topology_changed disabled", s.p.Capabilities().Platform)
		events = nil
	case err != nil:
		return err
	}
	samples, err := s.p.Throughput(ctx)
	switch {
	case errors.Is(err, provider.ErrUnsupported):
		s.logger.Printf("throughput not supported by provider %s; throughput_sample disabled", s.p.Capabilities().Platform)
		samples = nil
	case err != nil:
		return err
	}

	// Baseline for the insight diff. A failure here is not fatal: the
	// first successful refresh then reports every insight as added.
	var prev *Annotated
	if a, err := s.Snapshot(ctx); err == nil {
		prev = a
	} else if ctx.Err() == nil {
		s.logger.Printf("initial snapshot failed: %v", err)
	}

	var (
		pending  []model.TopologyEvent
		timer    *time.Timer
		fire     <-chan time.Time
		deadline time.Time
	)
	stopTimer := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
			fire = nil
		}
	}
	defer stopTimer()

	for {
		select {
		case <-ctx.Done():
			s.drain(events, samples)
			return nil

		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if len(pending) == 0 {
				deadline = time.Now().Add(maxDebounce)
			}
			pending = append(pending, ev)
			wait := s.debounce
			if until := time.Until(deadline); until < wait {
				wait = until
			}
			if wait < 0 {
				wait = 0
			}
			stopTimer()
			timer = time.NewTimer(wait)
			fire = timer.C

		case <-fire:
			timer, fire = nil, nil
			burst := pending
			pending = nil
			if a, err := s.refresh(ctx, burst, prev); err == nil {
				prev = a
			} else if ctx.Err() == nil {
				s.logger.Printf("refresh after %d hotplug event(s) failed: %v", len(burst), err)
			}

		case smp, ok := <-samples:
			if !ok {
				samples = nil
				continue
			}
			s.recordSample(smp)
			s.hub.broadcast(Event{Type: EventThroughputSample, At: time.Now(), Data: smp})
		}
	}
}

// drainTimeout bounds how long Run waits for provider streams to close
// after cancellation. The Windows provider needs up to ~5 s to stop its
// ETW session; without this wait the process can exit first and leave
// the session running in the kernel.
const drainTimeout = 8 * time.Second

// drain waits for the provider streams to close after cancellation so the
// provider can release OS resources before the process exits.
func (s *Service) drain(events <-chan model.TopologyEvent, samples <-chan model.ThroughputSample) {
	deadline := time.After(drainTimeout)
	for events != nil || samples != nil {
		select {
		case _, ok := <-events:
			if !ok {
				events = nil
			}
		case _, ok := <-samples:
			if !ok {
				samples = nil
			}
		case <-deadline:
			s.logger.Printf("provider streams did not close within %s; giving up", drainTimeout)
			return
		}
	}
}

// refresh discards the cached snapshot, takes a fresh one and broadcasts
// the topology change plus the insight diff against prev.
func (s *Service) refresh(ctx context.Context, burst []model.TopologyEvent, prev *Annotated) (*Annotated, error) {
	a, err := s.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	devices := 0
	a.Topology.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, _ *model.Device) { devices++ })
	s.hub.broadcast(Event{Type: EventTopologyChanged, At: now, Data: TopologyChanged{
		Events:      nonNilEvents(burst),
		CapturedAt:  a.Topology.CapturedAt,
		FetchedAt:   a.FetchedAt,
		Controllers: len(a.Topology.Controllers),
		Devices:     devices,
	}})

	var before []insight.Insight
	if prev != nil {
		before = prev.Insights
	}
	added, resolved := diffInsights(before, a.Insights)
	for _, in := range added {
		s.hub.broadcast(Event{Type: EventInsightAdded, At: now, Data: in})
	}
	for _, in := range resolved {
		s.hub.broadcast(Event{Type: EventInsightResolved, At: now, Data: in})
	}
	return a, nil
}

// Refresh invalidates the cache and takes a fresh snapshot from the
// provider. Any snapshot already in flight is allowed to finish first (its
// result may predate the change that prompted the refresh), and callers of
// Snapshot that arrive meanwhile share the fresh result.
func (s *Service) Refresh(ctx context.Context) (*Annotated, error) {
	for {
		s.mu.Lock()
		if c := s.inflight; c != nil {
			s.mu.Unlock()
			select {
			case <-c.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}
		c := &call{done: make(chan struct{})}
		s.inflight = c
		s.cached = nil
		s.mu.Unlock()

		go s.run(ctx, c)
		return c.wait(ctx)
	}
}

// insightKey identifies an insight across snapshots: rule id plus its
// device ids in sorted order.
func insightKey(in insight.Insight) string {
	ids := append([]string(nil), in.DeviceIDs...)
	sort.Strings(ids)
	return in.RuleID + "\x00" + strings.Join(ids, "\x00")
}

// diffInsights reports which insights appeared in after and which from
// before are gone, each in the order of its source slice.
func diffInsights(before, after []insight.Insight) (added, resolved []insight.Insight) {
	old := make(map[string]bool, len(before))
	for _, in := range before {
		old[insightKey(in)] = true
	}
	cur := make(map[string]bool, len(after))
	for _, in := range after {
		k := insightKey(in)
		cur[k] = true
		if !old[k] {
			added = append(added, in)
		}
	}
	for _, in := range before {
		if !cur[insightKey(in)] {
			resolved = append(resolved, in)
		}
	}
	return added, resolved
}

func nonNilEvents(in []model.TopologyEvent) []model.TopologyEvent {
	if in == nil {
		return []model.TopologyEvent{}
	}
	return in
}

// storedSample is a throughput sample with the time the service saw it,
// which is what the freshness window is measured against.
type storedSample struct {
	sample model.ThroughputSample
	seenAt time.Time
}

// recordSample remembers the latest sample for its device and forgets
// devices whose last sample has aged out.
func (s *Service) recordSample(smp model.ThroughputSample) {
	now := time.Now()
	s.samplesMu.Lock()
	defer s.samplesMu.Unlock()
	s.samples[smp.DeviceID] = storedSample{sample: smp, seenAt: now}
	for id, st := range s.samples {
		if now.Sub(st.seenAt) > sampleMaxAge {
			delete(s.samples, id)
		}
	}
}

// RecentSamples returns the latest throughput sample per device received
// within the freshness window, ordered by device id.
func (s *Service) RecentSamples() []model.ThroughputSample {
	now := time.Now()
	s.samplesMu.Lock()
	out := make([]model.ThroughputSample, 0, len(s.samples))
	for _, st := range s.samples {
		if now.Sub(st.seenAt) <= sampleMaxAge {
			out = append(out, st.sample)
		}
	}
	s.samplesMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
	return out
}
