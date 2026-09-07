//go:build windows

package platform

import (
	"portauthority/core/provider"
	"portauthority/platform/win"
)

func newNative() (provider.Provider, error) {
	return win.New(), nil
}
