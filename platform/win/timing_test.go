//go:build windows

package win

import (
	"context"
	"testing"
	"time"

	"portauthority/core/model"
)

// TestSnapshotTiming reports where a cold snapshot spends its time. It is
// informational (never fails) and only runs with -run Timing -v.
func TestSnapshotTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("timing only")
	}
	start := time.Now()
	idx, err := loadPnPIndex()
	t.Logf("pnp index: %s (err=%v, %d driver keys)", time.Since(start).Round(time.Millisecond), err, len(idx.byDriverKey))

	w := &walker{pnp: idx}
	start = time.Now()
	paths, err := deviceInterfaces(&guidUSBHostController)
	t.Logf("controller interfaces: %s (%d, err=%v)", time.Since(start).Round(time.Millisecond), len(paths), err)

	topo := &model.Topology{}
	for i, p := range paths {
		start = time.Now()
		c, err := w.collectController(context.Background(), i+1, p)
		t.Logf("controller %d walk: %s (err=%v)", i+1, time.Since(start).Round(time.Millisecond), err)
		topo.Controllers = append(topo.Controllers, c)
	}

	start = time.Now()
	w.collectUSB4(topo)
	t.Logf("usb4 collect: %s (%d routers)", time.Since(start).Round(time.Millisecond), len(topo.USB4))
	for _, warn := range w.warnings {
		t.Logf("warning: %s", warn)
	}
}
