package insight

import (
	"context"
	"strings"
	"testing"

	"portauthority/core/enrich"
	"portauthority/core/model"
	"portauthority/core/provider/mock"
)

func byRule(insights []Insight, id string) []Insight {
	var out []Insight
	for _, in := range insights {
		if in.RuleID == id {
			out = append(out, in)
		}
	}
	return out
}

func TestDeviantFixtureFindsTheSSDOnTheWrongPort(t *testing.T) {
	p, err := mock.Load("../../testdata/fixtures/deviant-caldigit-ts4.json")
	if err != nil {
		t.Fatal(err)
	}
	topo, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	enrich.Annotate(topo)
	insights := Evaluate(topo)

	faster := byRule(insights, "faster-port-available")
	if len(faster) != 1 {
		t.Fatalf("faster-port-available: got %d insights, want 1: %+v", len(faster), faster)
	}
	in := faster[0]
	for _, want := range []string{"Corsair EX400U", "faster"} {
		if !strings.Contains(in.Title, want) {
			t.Errorf("title %q lacks %q", in.Title, want)
		}
	}
	if !strings.Contains(in.Explanation, `"USB-C Data"`) || !strings.Contains(in.Explanation, "CalDigit TS4") {
		t.Errorf("explanation should name the labelled port and the dock: %q", in.Explanation)
	}
	if !strings.Contains(in.Suggestion, `"Thunderbolt 4"`) {
		t.Errorf("suggestion should point at the Thunderbolt ports: %q", in.Suggestion)
	}
	if in.Confidence < 0.9 {
		t.Errorf("mapped port should be high confidence, got %v", in.Confidence)
	}

	for _, id := range []string{"link-downgrade", "usb2-on-dock", "hub-chain-depth", "power-budget", "iso-reservation-squeeze"} {
		if got := byRule(insights, id); len(got) != 0 {
			t.Errorf("%s should not fire on the DEVIANT fixture: %+v", id, got)
		}
	}
}

func TestDeviantAfterMovingTheSSDIsQuiet(t *testing.T) {
	// Same desk after the EX400U moved to a Thunderbolt 4 port: it tunnels
	// PCIe and leaves the USB tree. The only thing left to say is the
	// positive confirmation from its USB4 router node.
	p, err := mock.Load("../../testdata/fixtures/deviant-caldigit-ts4-ssd-on-tb4.json")
	if err != nil {
		t.Fatal(err)
	}
	topo, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	enrich.Annotate(topo)
	insights := Evaluate(topo)
	if len(insights) != 1 {
		t.Fatalf("expected exactly one insight, got %d: %+v", len(insights), insights)
	}
	in := insights[0]
	if in.RuleID != "usb4-device-well-placed" || in.Severity != SeverityInfo {
		t.Fatalf("got %s/%s, want usb4-device-well-placed/info: %+v", in.RuleID, in.Severity, in)
	}
	if in.Title != "Corsair EX400U is on a 40 Gbps (USB4 / Thunderbolt) port" {
		t.Errorf("title = %q", in.Title)
	}
	if in.Suggestion != "Nothing to do." || in.Confidence != 0.8 {
		t.Errorf("suggestion %q confidence %v", in.Suggestion, in.Confidence)
	}
	if len(in.DeviceIDs) != 1 || !strings.HasPrefix(strings.ToUpper(in.DeviceIDs[0]), `USB4\VID_13FE&PID_6900`) {
		t.Errorf("device ids = %v", in.DeviceIDs)
	}
	var carriesDisk bool
	for _, e := range in.Evidence {
		if strings.HasPrefix(e, "carries ") && strings.Contains(e, "EX400U") {
			carriesDisk = true
		}
	}
	if !carriesDisk {
		t.Errorf("evidence should name the NVMe disk the router carries: %v", in.Evidence)
	}
}

func TestUSB4DeviceWellPlaced(t *testing.T) {
	topo := &model.Topology{SchemaVersion: 1, USB4: []model.USB4Router{
		{ID: "host", Kind: "host", VendorID: 0x8086, ProductID: 0xa76d},
		{ID: "ts4", Kind: "device", VendorID: 0x8087, ProductID: 0x0b26, ParentID: "host", Depth: 1},
		{ID: "ex400u", Kind: "device", VendorID: 0x13fe, ProductID: 0x6900, ParentID: "ts4", Depth: 2,
			Children: []string{`Corsair EX400U (SCSI\DISK&VEN_NVME&PROD_CORSAIR_EX400U\7&1B27237F&0&000000)`}},
	}}
	got := byRule(Evaluate(topo), "usb4-device-well-placed")
	if len(got) != 1 || !strings.Contains(got[0].Title, "Corsair EX400U") || got[0].DeviceIDs[0] != "ex400u" {
		t.Fatalf("got %+v", got)
	}
	// An unknown router, or one that is only a dock, stays quiet.
	topo.USB4 = topo.USB4[:2]
	if got := byRule(Evaluate(topo), "usb4-device-well-placed"); len(got) != 0 {
		t.Fatalf("fired without a known device: %+v", got)
	}
}

// --- synthetic topologies ---------------------------------------------

func device(id string, vid, pid uint16, class model.DeviceClass, claimed model.LinkSpeed, mA int) *model.Device {
	return &model.Device{ID: id, PortPath: id, VendorID: vid, ProductID: pid, Class: class, ClaimedSpeed: claimed, PowerDrawMA: mA, Product: id}
}

func hub(id string, vid, pid uint16, kind model.HubKind, busPowered bool, ports ...model.Port) *model.Device {
	d := device(id, vid, pid, model.ClassHub, model.LinkSuper, 0)
	d.Hub = &model.Hub{Kind: kind, BusPowered: busPowered, PortCount: len(ports), Ports: ports}
	return d
}

func port(n int, link, max model.LinkSpeed, d *model.Device) model.Port {
	return model.Port{Number: n, Status: model.StatusConnected, NegotiatedLink: link, MaxLink: max, Device: d}
}

func topology(root *model.Device) *model.Topology {
	root.Hub.Kind = model.HubRoot
	return &model.Topology{SchemaVersion: 1, Controllers: []model.Controller{{ID: "c1", Name: "Test xHCI", RootHub: root}}}
}

func TestLinkDowngrade(t *testing.T) {
	ssd := device("ssd", 0x1234, 0x0001, model.ClassStorage, model.LinkSuperPlus, 500)
	root := hub("root", 0, 0, model.HubRoot, false, port(1, model.LinkHigh, model.LinkSuperPlus, ssd))
	got := byRule(Evaluate(topology(root)), "link-downgrade")
	if len(got) != 1 || !strings.Contains(got[0].Title, "slower than it should") {
		t.Fatalf("got %+v", got)
	}

	// The USB 2 logical side of a USB 3 hub reports the physical device as
	// SuperSpeed-capable while its port can only do high speed. Not a downgrade.
	usb2side := hub("h2", 0x2188, 0x5802, model.HubUSB2, false)
	root = hub("root", 0, 0, model.HubRoot, false, port(1, model.LinkHigh, model.LinkHigh, usb2side))
	if got := byRule(Evaluate(topology(root)), "link-downgrade"); len(got) != 0 {
		t.Fatalf("companion hub artefact fired: %+v", got)
	}
}

func TestUSB2OnDock(t *testing.T) {
	cam := device("cam", 0x1234, 0x0002, model.ClassVideo, model.LinkSuper, 500)
	dockUSB2 := hub("ts4-usb2", 0x2188, 0x5802, model.HubUSB2, false, port(3, model.LinkHigh, model.LinkHigh, cam))
	root := hub("root", 0, 0, model.HubRoot, false, port(1, model.LinkHigh, model.LinkHigh, dockUSB2))
	got := byRule(Evaluate(topology(root)), "usb2-on-dock")
	if len(got) != 1 || !strings.Contains(got[0].Title, "CalDigit TS4") {
		t.Fatalf("got %+v", got)
	}
	// Same device on a plain hub that is not a known dock: stays quiet.
	plain := hub("plain", 0x0bda, 0x5411, model.HubUSB2, false, port(3, model.LinkHigh, model.LinkHigh, cam))
	root = hub("root", 0, 0, model.HubRoot, false, port(1, model.LinkHigh, model.LinkHigh, plain))
	if got := byRule(Evaluate(topology(root)), "usb2-on-dock"); len(got) != 0 {
		t.Fatalf("fired without a dock: %+v", got)
	}
}

func TestHubChainDepthCountsDocksOnce(t *testing.T) {
	mouse := device("mouse", 0x046d, 0xc52b, model.ClassHID, model.LinkFull, 100)
	h3 := hub("h3", 0x0bda, 0x5411, model.HubUSB2, true, port(1, model.LinkFull, model.LinkHigh, mouse))
	h2 := hub("h2", 0x0bda, 0x5411, model.HubUSB2, true, port(1, model.LinkHigh, model.LinkHigh, h3))
	h1 := hub("h1", 0x0bda, 0x5411, model.HubUSB2, false, port(1, model.LinkHigh, model.LinkHigh, h2))
	root := hub("root", 0, 0, model.HubRoot, false, port(1, model.LinkHigh, model.LinkHigh, h1))
	got := byRule(Evaluate(topology(root)), "hub-chain-depth")
	if len(got) != 1 || !strings.Contains(got[0].Title, "3 chained hubs") {
		t.Fatalf("got %+v", got)
	}

	// Three TS4-internal hubs are one hop.
	d3 := hub("d3", 0x2188, 0x5511, model.HubUSB2, false, port(1, model.LinkFull, model.LinkHigh, mouse))
	d2 := hub("d2", 0x2188, 0x5510, model.HubUSB2, false, port(1, model.LinkHigh, model.LinkHigh, d3))
	d1 := hub("d1", 0x2188, 0x5802, model.HubUSB2, false, port(1, model.LinkHigh, model.LinkHigh, d2))
	root = hub("root", 0, 0, model.HubRoot, false, port(1, model.LinkHigh, model.LinkHigh, d1))
	ctx := NewContext(topology(root))
	if hops := ctx.ByID["mouse"].HopCount(); hops != 1 {
		t.Fatalf("dock internals should count as one hop, got %d", hops)
	}
}

func TestPowerBudget(t *testing.T) {
	a := device("drive-a", 0x1234, 0x0003, model.ClassStorage, model.LinkHigh, 500)
	b := device("drive-b", 0x1234, 0x0004, model.ClassStorage, model.LinkHigh, 500)
	h := hub("cheap-hub", 0x0bda, 0x5411, model.HubUSB2, true, port(1, model.LinkHigh, model.LinkHigh, a), port(2, model.LinkHigh, model.LinkHigh, b))
	root := hub("root", 0, 0, model.HubRoot, false, port(1, model.LinkHigh, model.LinkHigh, h))
	got := byRule(Evaluate(topology(root)), "power-budget")
	if len(got) != 1 || !strings.Contains(got[0].Explanation, "1000 mA") {
		t.Fatalf("got %+v", got)
	}
	h.Hub.BusPowered = false
	if got := byRule(Evaluate(topology(root)), "power-budget"); len(got) != 0 {
		t.Fatalf("self-powered hub fired: %+v", got)
	}
}

func TestIsoReservationSqueeze(t *testing.T) {
	cam := device("cam", 0x1234, 0x0005, model.ClassVideo, model.LinkHigh, 500)
	cam.IsoReserved = 200 * model.Mbps
	stick := device("stick", 0x1234, 0x0006, model.ClassStorage, model.LinkHigh, 200)
	root := hub("root", 0, 0, model.HubRoot, false,
		port(1, model.LinkHigh, model.LinkHigh, cam),
		port(2, model.LinkHigh, model.LinkHigh, stick))
	got := byRule(Evaluate(topology(root)), "iso-reservation-squeeze")
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	cam.IsoReserved = 2 * model.Mbps
	if got := byRule(Evaluate(topology(root)), "iso-reservation-squeeze"); len(got) != 0 {
		t.Fatalf("tiny reservation fired: %+v", got)
	}
}

func TestInsightsAreOrderedBySeverity(t *testing.T) {
	ins := EvaluateWith(&Context{}, []Rule{
		{ID: "b", Evaluate: func(*Context) []Insight { return []Insight{{Severity: SeverityInfo, Title: "info"}} }},
		{ID: "a", Evaluate: func(*Context) []Insight { return []Insight{{Severity: SeverityWarning, Title: "warn"}} }},
	})
	if len(ins) != 2 || ins[0].Severity != SeverityWarning || ins[1].RuleID != "b" {
		t.Fatalf("got %+v", ins)
	}
}
