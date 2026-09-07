package kb

import (
	"testing"

	"portauthority/core/model"
)

func TestDockForHubFindsTS4(t *testing.T) {
	for _, pid := range []uint16{0x5802, 0x5500, 0x5501, 0x5512} {
		d, ok := DockForHub(0x2188, pid)
		if !ok || d.ID != "caldigit-ts4" {
			t.Fatalf("hub 2188:%04x: got %v %v, want caldigit-ts4", pid, d, ok)
		}
	}
	if _, ok := DockForHub(0x2188, 0x4042); ok {
		t.Error("CalDigit Pro Audio is not a TS4 hub")
	}
	if !IsGenericDockHub(0x8087, 0x0b40) {
		t.Error("Goshen Ridge hub should be generic")
	}
}

func TestTS4PortsAndMap(t *testing.T) {
	d, _ := DockForHub(0x2188, 0x5501)
	if d.BestLink() != model.LinkUSB4Gen3 {
		t.Errorf("best link = %s, want usb4_40", d.BestLink())
	}
	fast := d.PortsAtLeast(model.LinkUSB4Gen3)
	if len(fast) != 1 || fast[0].Label != "Thunderbolt 4" || fast[0].Count != 2 {
		t.Errorf("fast ports = %+v", fast)
	}
	m, ok := d.PhysicalPort(0x2188, 0x5501, 3)
	if !ok || m.Label != "USB-C Data" || m.Position != "front" {
		t.Errorf("port map = %+v %v", m, ok)
	}
	if _, ok := d.PhysicalPort(0x2188, 0x5501, 1); ok {
		t.Error("unmapped port should not resolve")
	}
}

func TestKnownDevice(t *testing.T) {
	dev, ok := KnownDeviceInfo(0x1b1c, 0x1a20)
	if !ok || dev.MaxLink != model.LinkUSB4Gen3 || dev.Kind != "storage" {
		t.Fatalf("EX400U = %+v %v", dev, ok)
	}
	if _, ok := KnownDeviceInfo(0xffff, 0xffff); ok {
		t.Error("unknown device should not resolve")
	}
}

func TestKnownDeviceByUSB4ID(t *testing.T) {
	dev, ok := KnownDeviceByUSB4ID(0x13fe, 0x6900)
	if !ok || dev.Name != "Corsair EX400U" || dev.MaxLink != model.LinkUSB4Gen3 {
		t.Fatalf("EX400U by USB4 id = %+v %v", dev, ok)
	}
	usb, _ := KnownDeviceInfo(0x1b1c, 0x1a20)
	if usb != dev {
		t.Error("USB and USB4 aliases should resolve to the same entry")
	}
	if _, ok := KnownDeviceByUSB4ID(0x1b1c, 0x1a20); ok {
		t.Error("USB vid:pid must not resolve in the USB4 namespace")
	}
}
