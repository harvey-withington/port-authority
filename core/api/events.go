package api

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"portauthority/core/insight"
	"portauthority/core/model"
)

// Event type names carried on the /api/v1/stream WebSocket. Every name the
// stream can emit is defined here and nowhere else.
const (
	// EventHello is sent once on connect with the schema version, provider
	// capabilities and the current insights.
	EventHello = "hello"
	// EventTopologyChanged follows a (debounced) burst of hotplug events
	// once a fresh snapshot has been taken.
	EventTopologyChanged = "topology_changed"
	// EventInsightAdded carries an insight that is present in the new
	// snapshot and was not in the previous one.
	EventInsightAdded = "insight_added"
	// EventInsightResolved carries an insight that was in the previous
	// snapshot and is gone from the new one.
	EventInsightResolved = "insight_resolved"
	// EventThroughputSample forwards one live throughput reading.
	EventThroughputSample = "throughput_sample"
)

// EventTypes lists every event type the stream can emit, in a stable order.
var EventTypes = []string{
	EventHello,
	EventTopologyChanged,
	EventInsightAdded,
	EventInsightResolved,
	EventThroughputSample,
}

// Event is one message on the stream: {"type": ..., "at": RFC3339, "data": ...}.
type Event struct {
	Type string    `json:"type"`
	At   time.Time `json:"at"`
	Data any       `json:"data"`
}

// Hello is the data of the EventHello event.
type Hello struct {
	SchemaVersion int                `json:"schema_version"`
	Capabilities  model.ProviderCaps `json:"capabilities"`
	// Types is the filter in effect for this subscriber; empty means all.
	Types []string `json:"types,omitempty"`
	// CapturedAt is the timestamp of the snapshot the insights come from.
	// It is zero and Error is set when no snapshot could be taken.
	CapturedAt time.Time         `json:"captured_at"`
	Insights   []insight.Insight `json:"insights"`
	Error      string            `json:"error,omitempty"`
}

// TopologyChanged is the data of the EventTopologyChanged event.
type TopologyChanged struct {
	// Events are the provider notifications that triggered the refresh.
	Events     []model.TopologyEvent `json:"events"`
	CapturedAt time.Time             `json:"captured_at"`
	// FetchedAt is when the fresh snapshot was requested.
	FetchedAt   time.Time `json:"fetched_at"`
	Controllers int       `json:"controllers"`
	Devices     int       `json:"devices"`
}

// subscriberBuffer is how many events a subscriber may lag before events
// are dropped for it.
const subscriberBuffer = 256

// hub fans events out to subscribers without ever blocking the producer.
type hub struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

// subscriber is one consumer of the event stream.
type subscriber struct {
	ch      chan Event
	types   map[string]bool // nil means every type
	dropped atomic.Int64
}

func newHub() *hub {
	return &hub{subs: map[*subscriber]struct{}{}}
}

// subscribe registers a consumer. An empty types list means every event.
func (h *hub) subscribe(types []string) *subscriber {
	s := &subscriber{ch: make(chan Event, subscriberBuffer)}
	if len(types) > 0 {
		s.types = map[string]bool{}
		for _, t := range types {
			s.types[t] = true
		}
	}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// unsubscribe removes a consumer. It is idempotent and never closes the
// channel, so a consumer racing with a broadcast cannot panic the producer.
func (h *hub) unsubscribe(s *subscriber) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
}

// broadcast delivers e to every interested subscriber. A subscriber whose
// buffer is full has the event dropped and counted.
func (h *hub) broadcast(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.types != nil && !s.types[e.Type] {
			continue
		}
		select {
		case s.ch <- e:
		default:
			s.dropped.Add(1)
		}
	}
}

// count reports the number of live subscribers.
func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Subscribe returns a channel of events for in-process consumers, filtered
// to the given types (none means all), and a function that ends the
// subscription. The channel is buffered; a consumer that falls behind
// loses events rather than stalling the service.
func (s *Service) Subscribe(types ...string) (<-chan Event, func()) {
	sub := s.hub.subscribe(types)
	return sub.ch, func() { s.hub.unsubscribe(sub) }
}

// parseTypes splits a comma-separated ?types= filter, dropping blanks and
// unknown names. It returns nil when nothing valid remains, which means
// "all".
func parseTypes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	known := map[string]bool{}
	for _, t := range EventTypes {
		known[t] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(part)
		if t == "" || !known[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
