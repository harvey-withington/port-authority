//go:build !windows

package platform

import "portauthority/core/provider"

func newNative() (provider.Provider, error) {
	return nil, provider.ErrUnsupported
}
