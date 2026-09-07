// Package api is the local HTTP API for Port Authority. It wraps a
// provider.Provider in a Service that caches an annotated snapshot
// (knowledge-base enrichment plus insights) and serves it under /api/v1.
//
// The server only ever binds to loopback; token authentication for
// third-party consumers is a later phase.
package api

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	"portauthority/core/enrich"
	"portauthority/core/insight"
	"portauthority/core/model"
	"portauthority/core/provider"
)

// SchemaVersion is stamped on every API response.
const SchemaVersion = model.SchemaVersion

// snapshotTimeout bounds one provider snapshot, independent of any
// individual caller's context, because a snapshot is shared by every
// caller waiting on it.
const snapshotTimeout = 30 * time.Second

// Annotated is a topology after knowledge-base enrichment, together with
// the insights evaluated against it. Instances are shared between callers
// of Service.Snapshot and must be treated as read-only.
type Annotated struct {
	Topology *model.Topology
	Insights []insight.Insight
	// FetchedAt is when the provider was asked for this snapshot. The
	// topology's own CapturedAt is what the provider reports and may be
	// older when replaying a fixture.
	FetchedAt time.Time
}

// Service produces annotated snapshots on demand and caches them.
type Service struct {
	p      provider.Provider
	ttl    time.Duration
	logger *log.Logger

	mu       sync.Mutex
	cached   *Annotated
	inflight *call

	// Live side, driven by Run.
	hub       *hub
	debounce  time.Duration
	samplesMu sync.Mutex
	samples   map[string]storedSample
}

// call is one in-flight provider snapshot shared by concurrent callers.
type call struct {
	done chan struct{}
	res  *Annotated
	err  error
}

// Option configures a Service.
type Option func(*Service)

// WithCacheTTL sets how long a snapshot is served from cache before the
// provider is asked again. The default is 30 seconds; Run refreshes on
// hotplug regardless of the TTL. A zero or negative TTL disables caching.
func WithCacheTTL(d time.Duration) Option {
	return func(s *Service) { s.ttl = d }
}

// WithLogger sets the logger used by the HTTP request log. By default
// requests are not logged.
func WithLogger(l *log.Logger) Option {
	return func(s *Service) {
		if l != nil {
			s.logger = l
		}
	}
}

// NewService wraps a provider.
func NewService(p provider.Provider, opts ...Option) *Service {
	s := &Service{
		p: p,
		// Topology only changes on hotplug, and Run re-snapshots on hotplug,
		// so a long TTL is safe. A cold snapshot on a busy dock takes
		// seconds, which is far too slow to repeat per request.
		ttl:      30 * time.Second,
		logger:   log.New(io.Discard, "", 0),
		hub:      newHub(),
		debounce: defaultDebounce,
		samples:  map[string]storedSample{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Capabilities reports what the underlying provider can measure.
func (s *Service) Capabilities() model.ProviderCaps {
	return s.p.Capabilities()
}

// Snapshot returns the current annotated topology. A snapshot younger than
// the cache TTL is returned as is; otherwise the provider is asked once,
// and every caller that arrives while that request is in flight shares
// its result. The returned value is shared and must not be mutated.
func (s *Service) Snapshot(ctx context.Context) (*Annotated, error) {
	s.mu.Lock()
	if a := s.cached; a != nil && time.Since(a.FetchedAt) < s.ttl {
		s.mu.Unlock()
		return a, nil
	}
	if c := s.inflight; c != nil {
		s.mu.Unlock()
		return c.wait(ctx)
	}
	c := &call{done: make(chan struct{})}
	s.inflight = c
	s.mu.Unlock()

	go s.run(ctx, c)
	return c.wait(ctx)
}

// run performs one provider snapshot and publishes the result to every
// waiter. It runs detached from the first caller's context so that a
// caller leaving early does not fail the others.
func (s *Service) run(ctx context.Context, c *call) {
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), snapshotTimeout)
	defer cancel()

	fetched := time.Now()
	t, err := s.p.Snapshot(sctx)
	if err == nil && t == nil {
		err = provider.ErrUnsupported
	}
	if err == nil {
		enrich.Annotate(t)
		c.res = &Annotated{
			Topology:  t,
			Insights:  insight.Evaluate(t),
			FetchedAt: fetched,
		}
	} else {
		c.err = err
	}

	s.mu.Lock()
	s.inflight = nil
	if c.err == nil {
		s.cached = c.res
	}
	s.mu.Unlock()
	close(c.done)
}

func (c *call) wait(ctx context.Context) (*Annotated, error) {
	select {
	case <-c.done:
		return c.res, c.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
