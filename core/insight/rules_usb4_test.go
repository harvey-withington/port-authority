package insight

import (
	"strings"
	"testing"

	"portauthority/core/enrich"
	"portauthority/core/model"
)

func hubDev(id string, vid, pid uint16, children ...*model.Device) *model.Device {
	ports := make([]model.Port, 0, len(children))
	for i, c := range children {
		ports = append(ports, model.Port{Number: i + 1, Device: c})
	}
	return &model.Device{ID: id, VendorID: vid, ProductID: pid, Hub: &model.Hub{PortCount: len(ports) + 1, Ports: ports}}
}

func ts4Router(link model.LinkSpeed, gen, lanes int) model.USB4Router {
	return model.USB4Router{
		ID: "r1", InstanceID: "r1", Name: "USB4 Router (1.0), CalDigit. Inc. - TS4",
		VendorID: 0x8087, ProductID: 0x0b26, Kind: "device", Vendor: "CalDigit, Inc.", Model: "TS4",
		LinkGen: gen, LinkLanes: lanes, NegotiatedLink: link,
	}
}

func TestUSB4LinkBelowMaxFlagsADockOnA20GbpsCable(t *testing.T) {
	topo := &model.Topology{
		Controllers: []model.Controller{{ID: "c1", RootHub: hubDev("root", 0, 0, hubDev("ts4", 0x2188, 0x5500))}},
		USB4:        []model.USB4Router{ts4Router(model.LinkUSB4Gen2, 2, 2)},
	}
	enrich.Annotate(topo)
	found := byRule(Evaluate(topo), "usb4-link-below-max")
	if len(found) != 1 {
		t.Fatalf("got %d insights, want 1: %+v", len(found), found)
	}
	in := found[0]
	if !strings.Contains(in.Title, "CalDigit TS4") || !strings.Contains(in.Title, "20 Gbps") {
		t.Errorf("title = %q", in.Title)
	}
	if in.Severity != SeverityWarning || in.DeviceIDs[0] != "r1" {
		t.Errorf("severity %s, ids %v", in.Severity, in.DeviceIDs)
	}
	if !strings.Contains(in.Suggestion, "cable") {
		t.Errorf("suggestion should talk about the cable: %q", in.Suggestion)
	}
}

func TestUSB4LinkBelowMaxStaysQuietAtFullSpeedOrWhenUnknown(t *testing.T) {
	for name, r := range map[string]model.USB4Router{
		"full speed": ts4Router(model.LinkUSB4Gen3, 3, 2),
		"no link":    ts4Router(model.LinkUnknown, 0, 0),
		"unknown product": {
			ID: "r2", Name: "USB4 Router (1.0), Acme - Dock9", VendorID: 0x8087, ProductID: 0x0b26, Kind: "device",
			Vendor: "Acme", Model: "Dock9", LinkGen: 2, LinkLanes: 2, NegotiatedLink: model.LinkUSB4Gen2,
		},
	} {
		topo := &model.Topology{USB4: []model.USB4Router{r}}
		enrich.Annotate(topo)
		if found := byRule(Evaluate(topo), "usb4-link-below-max"); len(found) != 0 {
			t.Errorf("%s: got %+v, want nothing", name, found)
		}
	}
}

func TestDockNotRecognisedNamesTheChainsAndTheRouter(t *testing.T) {
	// An unknown dock: two hub chains (its USB 2 and USB 3 halves) and a
	// router no dock claims.
	topo := &model.Topology{
		Controllers: []model.Controller{
			{ID: "c1", RootHub: hubDev("root1", 0, 0, hubDev("u2a", 0x1234, 0x0001, hubDev("u2b", 0x1234, 0x0002)))},
			{ID: "c2", RootHub: hubDev("root2", 0, 0, hubDev("u3a", 0x1234, 0x0003, hubDev("u3b", 0x1234, 0x0004)))},
		},
		USB4: []model.USB4Router{{
			ID: "rx", Name: "USB4 Router (1.0), Acme - Dock9", VendorID: 0x8087, ProductID: 0x0b26, Kind: "device",
			Vendor: "Acme", Model: "Dock9",
		}},
	}
	enrich.Annotate(topo)
	found := byRule(Evaluate(topo), "dock-not-recognised")
	if len(found) != 1 {
		t.Fatalf("got %d insights, want 1: %+v", len(found), found)
	}
	in := found[0]
	if !strings.HasPrefix(in.Title, "Acme Dock9 looks like a dock") {
		t.Errorf("title = %q", in.Title)
	}
	want := []string{"u2a", "u2b", "u3a", "u3b", "rx"}
	if strings.Join(in.DeviceIDs, ",") != strings.Join(want, ",") {
		t.Errorf("device ids = %v, want %v", in.DeviceIDs, want)
	}
}

func TestDockNotRecognisedIgnoresKnownDocksAndLoneHubs(t *testing.T) {
	topo := &model.Topology{
		Controllers: []model.Controller{
			// A known dock and a plain single hub with a mouse.
			{ID: "c1", RootHub: hubDev("root1", 0, 0, hubDev("ts4", 0x2188, 0x5500, hubDev("ts4b", 0x2188, 0x5501)))},
			{ID: "c2", RootHub: hubDev("root2", 0, 0, hubDev("lone", 0x0bda, 0x0411, &model.Device{ID: "mouse", VendorID: 0x046d, ProductID: 0xc52b}))},
		},
	}
	enrich.Annotate(topo)
	if found := byRule(Evaluate(topo), "dock-not-recognised"); len(found) != 0 {
		t.Errorf("got %+v, want nothing", found)
	}
}
