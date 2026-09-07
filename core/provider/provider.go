// Package provider defines the boundary between the platform-neutral core
// and OS-specific collectors. Exactly one Provider is active at runtime.
package provider

import (
	"context"
	"errors"

	"portauthority/core/model"
)

// ErrUnsupported is returned by a provider for any capability it does not
// declare in Capabilities().
var ErrUnsupported = errors.New("provider: capability not supported on this platform")

// Provider is implemented once per OS. Consumers must consult
// Capabilities() rather than the OS name to decide what to render.
//
// Stream contract: Watch and Throughput return channels that the provider
// closes once ctx is cancelled and every OS resource behind the stream
// has been released. Consumers wait for that close before exiting; on
// Windows the throughput stream owns an ETW session that would otherwise
// outlive the process.
type Provider interface {
	// Snapshot returns the full topology: controllers -> hubs -> ports -> devices.
	Snapshot(ctx context.Context) (*model.Topology, error)

	// Watch streams hotplug / link-change events until ctx is cancelled,
	// then closes the channel.
	Watch(ctx context.Context) (<-chan model.TopologyEvent, error)

	// Throughput streams live per-device throughput, best effort, until
	// ctx is cancelled, then closes the channel.
	Throughput(ctx context.Context) (<-chan model.ThroughputSample, error)

	// Capabilities declares what this provider can actually measure.
	Capabilities() model.ProviderCaps
}
