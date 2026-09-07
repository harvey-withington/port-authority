// Package mock is a Provider that replays a serialized Topology fixture.
// It drives the whole app without hardware and is the development path
// for platforms that have no real provider yet.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"portauthority/core/model"
	"portauthority/core/provider"
)

// Provider replays a fixed topology.
type Provider struct {
	topology *model.Topology
}

// New wraps an in-memory topology.
func New(t *model.Topology) *Provider {
	return &Provider{topology: t}
}

// Load reads a JSON topology fixture from disk.
func Load(path string) (*Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mock: read fixture: %w", err)
	}
	var t model.Topology
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("mock: parse fixture %s: %w", path, err)
	}
	return New(&t), nil
}

// Snapshot returns a deep copy so callers can mutate freely.
func (p *Provider) Snapshot(context.Context) (*model.Topology, error) {
	data, err := json.Marshal(p.topology)
	if err != nil {
		return nil, err
	}
	var copy model.Topology
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}

// Watch never emits; the channel closes when ctx is done.
func (p *Provider) Watch(ctx context.Context) (<-chan model.TopologyEvent, error) {
	ch := make(chan model.TopologyEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// Throughput is not supported by the mock.
func (p *Provider) Throughput(context.Context) (<-chan model.ThroughputSample, error) {
	return nil, provider.ErrUnsupported
}

// Capabilities reports topology only.
func (p *Provider) Capabilities() model.ProviderCaps {
	return model.ProviderCaps{Platform: "mock", Topology: true}
}
