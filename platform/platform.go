// Package platform selects the Provider for the OS this binary runs on.
// It is the only package allowed to import platform-specific providers;
// core, api and UI code depend on core/provider alone.
package platform

import "portauthority/core/provider"

// New returns the native provider for this OS, or provider.ErrUnsupported.
func New() (provider.Provider, error) {
	return newNative()
}
