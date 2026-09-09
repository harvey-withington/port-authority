package enrich

import (
	"encoding/json"
	"os"
	"testing"

	"portauthority/core/model"
)

// TS4 hub IDs from docks.json, and the Intel chipset hub that sits inside
// most Thunderbolt 4 docks.
const (
	ts4USB2Top = 0x5802
	ts4USB2Mid = 0x5510
	ts4USB3Top = 0x5500
	ts4USB3Mid = 0x5501
	ts4Vendor  = 0x2188
	goshenVID  = 0x8087
	goshenPID  = 0x0b40
)

func hub(id string, vid, pid uint16, children ...*model.Device) *model.Device {
	ports := make([]model.Port, 0, len(children))
	for i, c := range children {
		ports = append(ports, model.Port{Number: i + 1, Device: c})
	}
	return &model.Device{ID: id, VendorID: vid, ProductID: pid, Hub: &model.Hub{PortCount: len(ports) + 1, Ports: ports}}
}

// encOf returns the enclosure ID of the named hub, or "" when it has none.
func encOf(t *testing.T, topo *model.Topology, id string) string {
	t.Helper()
	found := ""
	topo.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		if d.ID == id && d.Enclosure != nil {
			found = d.Enclosure.ID
		}
	})
	return found
}

func TestEnclosureHostIsEveryRootHub(t *testing.T) {
	topo := &model.Topology{Controllers: []model.Controller{
		{ID: "c1", RootHub: hub("root1", 0, 0)},
		{ID: "c2", RootHub: hub("root2", 0, 0)},
	}}
	Annotate(topo)

	for _, id := range []string{"root1", "root2"} {
		if got := encOf(t, topo, id); got != "host" {
			t.Errorf("%s enclosure = %q, want host", id, got)
		}
	}
}

func TestEnclosureFoldsDockChainAcrossControllers(t *testing.T) {
	// The shape Windows reports for one TS4: its USB 2 hubs hang off one
	// controller and its USB 3 hubs off another, behind a chipset hub.
	usb2 := hub("d2a", ts4Vendor, ts4USB2Top, hub("d2b", ts4Vendor, ts4USB2Mid))
	usb3 := hub("goshen", goshenVID, goshenPID, hub("d3a", ts4Vendor, ts4USB3Top, hub("d3b", ts4Vendor, ts4USB3Mid)))
	topo := &model.Topology{Controllers: []model.Controller{
		{ID: "c1", RootHub: hub("root1", 0, 0, usb2)},
		{ID: "c2", RootHub: hub("root2", 0, 0, usb3)},
	}}
	Annotate(topo)

	want := encOf(t, topo, "d2a")
	if want != "dock:caldigit-ts4:1" {
		t.Fatalf("first dock hub enclosure = %q, want dock:caldigit-ts4:1", want)
	}
	// Both halves and the chipset hub are one box.
	for _, id := range []string{"d2b", "goshen", "d3a", "d3b"} {
		if got := encOf(t, topo, id); got != want {
			t.Errorf("%s enclosure = %q, want %q", id, got, want)
		}
	}
	if got := encOf(t, topo, "root1"); got != "host" {
		t.Errorf("root1 enclosure = %q, want host", got)
	}
}

func TestEnclosureKeepsTwoDocksOfTheSameModelApart(t *testing.T) {
	// A repeated hub VID:PID is a second dock, not more of the first.
	a := hub("a", ts4Vendor, ts4USB3Top, hub("a2", ts4Vendor, ts4USB3Mid))
	b := hub("b", ts4Vendor, ts4USB3Top, hub("b2", ts4Vendor, ts4USB3Mid))
	topo := &model.Topology{Controllers: []model.Controller{
		{ID: "c1", RootHub: hub("root", 0, 0, a, b)},
	}}
	Annotate(topo)

	first, second := encOf(t, topo, "a"), encOf(t, topo, "b")
	if first == second {
		t.Fatalf("both docks got enclosure %q, want distinct boxes", first)
	}
	if got := encOf(t, topo, "a2"); got != first {
		t.Errorf("a2 enclosure = %q, want %q", got, first)
	}
	if got := encOf(t, topo, "b2"); got != second {
		t.Errorf("b2 enclosure = %q, want %q", got, second)
	}
}

func TestEnclosureLeavesUnknownHubsAlone(t *testing.T) {
	// An unrecognised hub is its own box: nesting alone never merges.
	inner := hub("inner", 0x2109, 0x2817)
	outer := hub("outer", 0x05e3, 0x0610, inner)
	topo := &model.Topology{Controllers: []model.Controller{
		{ID: "c1", RootHub: hub("root", 0, 0, outer)},
	}}
	Annotate(topo)

	if outerEnc, innerEnc := encOf(t, topo, "outer"), encOf(t, topo, "inner"); outerEnc == innerEnc {
		t.Errorf("unrelated hubs share enclosure %q, want one box each", outerEnc)
	}
	if got := encOf(t, topo, "outer"); got != "hub:outer" {
		t.Errorf("outer enclosure = %q, want hub:outer", got)
	}
}

func TestEnclosureLoneChipsetHubIsItsOwnBox(t *testing.T) {
	// Without a dock hub next to it there is nothing to say the chipset
	// hub is inside a dock, so it stays a box of its own.
	topo := &model.Topology{Controllers: []model.Controller{
		{ID: "c1", RootHub: hub("root", 0, 0, hub("goshen", goshenVID, goshenPID))},
	}}
	Annotate(topo)

	if got := encOf(t, topo, "goshen"); got != "hub:goshen" {
		t.Errorf("lone chipset hub enclosure = %q, want hub:goshen", got)
	}
}

// The real capture is the check that matters: on DEVIANT the TS4 reports
// eight logical hubs over two controllers and must come out as one box.
func TestEnclosureOnCalDigitFixture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/fixtures/deviant-caldigit-ts4.json")
	if err != nil {
		t.Fatal(err)
	}
	var topo model.Topology
	if err := json.Unmarshal(raw, &topo); err != nil {
		t.Fatal(err)
	}
	Annotate(&topo)

	boxes := map[string]int{}
	topo.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		if d.Hub == nil {
			return
		}
		if d.Enclosure == nil {
			t.Errorf("hub %s has no enclosure", d.ID)
			return
		}
		boxes[d.Enclosure.ID]++
	})

	if len(boxes) != 2 {
		t.Fatalf("boxes = %v, want exactly host and one dock", boxes)
	}
	if boxes["host"] != 2 {
		t.Errorf("host holds %d hubs, want the 2 root hubs", boxes["host"])
	}
	if boxes["dock:caldigit-ts4:1"] != 8 {
		t.Errorf("dock holds %d hubs, want 8", boxes["dock:caldigit-ts4:1"])
	}
}

// hubWithPath is a hub whose device path other hubs' connectors can name.
func hubWithPath(id, path string, vid, pid uint16, ports ...model.Port) *model.Device {
	return &model.Device{ID: id, VendorID: vid, ProductID: pid,
		Hub: &model.Hub{PortCount: len(ports) + 1, DevicePath: path, Ports: ports}}
}

// companionPort is a port whose connector names the other half of the same
// physical socket, the way IOCTL_USB_GET_PORT_CONNECTOR_PROPERTIES does.
func companionPort(n int, dev *model.Device, peerPath string, peerPort int) model.Port {
	return model.Port{Number: n, Device: dev,
		Connector: &model.Connector{TypeC: true, UserConnectable: true, CompanionHubPath: peerPath, CompanionPort: peerPort}}
}

func TestEnclosureFoldsTheTwoHalvesOfOneHub(t *testing.T) {
	// A plain USB-C hub: its USB 3 half enumerates on one controller and
	// its USB 2 half on another, and Windows links the two sockets. Nothing
	// here is in the knowledge base, so only the companion link can say
	// these are one hub.
	const rootAPath, rootBPath = `USB#ROOT_HUB30#A#{g}`, `USB#ROOT_HUB30#B#{g}`
	usb3 := hub("via3", 0x2109, 0x0822)
	usb2 := hub("via2", 0x2109, 0x2822)
	rootA := hubWithPath("rootA", rootAPath, 0, 0, companionPort(2, usb3, rootBPath, 3))
	rootB := hubWithPath("rootB", rootBPath, 0, 0, companionPort(3, usb2, rootAPath, 2))
	topo := &model.Topology{Controllers: []model.Controller{
		{ID: "c1", RootHub: rootA},
		{ID: "c2", RootHub: rootB},
	}}
	Annotate(topo)

	got := encOf(t, topo, "via3")
	if got == "" || got != encOf(t, topo, "via2") {
		t.Fatalf("halves got %q and %q, want one box", got, encOf(t, topo, "via2"))
	}
	if got != "hub:via3" {
		t.Errorf("box id = %q, want hub:via3 (the half walked first)", got)
	}
}

func TestEnclosureIgnoresACompanionThatIsNotAHub(t *testing.T) {
	// The other half of the socket holds a plain device, so there is no
	// second hub to fold and the hub stays a box of its own.
	const rootAPath, rootBPath = `USB#ROOT_HUB30#A#{g}`, `USB#ROOT_HUB30#B#{g}`
	lone := hub("lone", 0x2109, 0x0822)
	mouse := &model.Device{ID: "mouse", VendorID: 0x046d, ProductID: 0xc52b}
	rootA := hubWithPath("rootA", rootAPath, 0, 0, companionPort(2, lone, rootBPath, 3))
	rootB := hubWithPath("rootB", rootBPath, 0, 0, companionPort(3, mouse, rootAPath, 2))
	topo := &model.Topology{Controllers: []model.Controller{
		{ID: "c1", RootHub: rootA},
		{ID: "c2", RootHub: rootB},
	}}
	Annotate(topo)

	if got := encOf(t, topo, "lone"); got != "hub:lone" {
		t.Errorf("enclosure = %q, want hub:lone", got)
	}
}

func TestEnclosureRecomputesGroupingsAlreadyInTheSnapshot(t *testing.T) {
	// An enclosure is derived, not observed. Replaying a capture that
	// already carries one has to recompute it, or a grouping fix could
	// never reach an old snapshot.
	root := hub("root", 0, 0)
	root.Enclosure = &model.Enclosure{ID: "stale", Kind: model.EnclosureDock}
	topo := &model.Topology{Controllers: []model.Controller{{ID: "c1", RootHub: root}}}
	Annotate(topo)

	if root.Enclosure.ID != "host" {
		t.Errorf("enclosure = %q, want it recomputed to host", root.Enclosure.ID)
	}
}

func TestAnnotateNamesUSB4RoutersFromTheKnowledgeBase(t *testing.T) {
	// A router reports its bridge silicon; the knowledge base knows the
	// product that silicon is inside.
	topo := &model.Topology{USB4: []model.USB4Router{
		{ID: "r1", Name: "USB4 Router (2.0), Phison - PS2321", VendorID: 0x13fe, ProductID: 0x6900, Kind: "device"},
		{ID: "r2", Name: "USB4 Root Router (1.0)", VendorID: 0x8086, ProductID: 0xe433, Kind: "host"},
	}}
	Annotate(topo)

	if topo.USB4[0].ProductName != "Corsair EX400U" {
		t.Errorf("device router ProductName = %q, want Corsair EX400U", topo.USB4[0].ProductName)
	}
	if topo.USB4[0].Name != "USB4 Router (2.0), Phison - PS2321" {
		t.Errorf("Name = %q, want the OS name kept", topo.USB4[0].Name)
	}
	if topo.USB4[1].ProductName != "" {
		t.Errorf("unknown router ProductName = %q, want empty", topo.USB4[1].ProductName)
	}
}

// internalPort is a port the firmware says is not a socket: a chip wired
// inside the box rather than something a person plugged in.
func internalPort(n int, dev *model.Device) model.Port {
	return model.Port{Number: n, Device: dev,
		Connector: &model.Connector{UserConnectable: false}}
}

// socketPort is a port the firmware says is a real socket.
func socketPort(n int, dev *model.Device) model.Port {
	return model.Port{Number: n, Device: dev,
		Connector: &model.Connector{UserConnectable: true}}
}

func TestEnclosureFoldsAHubWiredToAnInternalPort(t *testing.T) {
	// A hub whose firmware fills in its port table: the cascade to the
	// second chip is marked as not a socket, so both chips are one box.
	second := hub("second", 0x05e3, 0x0610)
	first := &model.Device{ID: "first", VendorID: 0x2109, ProductID: 0x0822,
		Hub: &model.Hub{PortCount: 3, Ports: []model.Port{internalPort(1, second), socketPort(2, nil)}}}
	root := hub("root", 0, 0)
	root.Hub.Ports = []model.Port{socketPort(1, first)}
	topo := &model.Topology{Controllers: []model.Controller{{ID: "c1", RootHub: root}}}
	Annotate(topo)

	if got, want := encOf(t, topo, "second"), encOf(t, topo, "first"); got != want {
		t.Errorf("second chip enclosure = %q, want %q", got, want)
	}
	// The hub itself is still plugged into the computer, not part of it.
	if encOf(t, topo, "first") == "host" {
		t.Error("the hub folded into the computer, want a box of its own")
	}
}

func TestEnclosureKeepsHubsApartWhenTheFirmwareClaimsEveryPortIsASocket(t *testing.T) {
	// The common cheap-hub case, measured on real hardware: the port table
	// is unfilled so an internal cascade claims to be user connectable.
	// Nothing can tell it from a hub someone plugged in, so it stays its
	// own box rather than being folded on a guess.
	second := hub("second", 0x05e3, 0x0610)
	first := &model.Device{ID: "first", VendorID: 0x2109, ProductID: 0x0822,
		Hub: &model.Hub{PortCount: 3, Ports: []model.Port{socketPort(1, second)}}}
	root := hub("root", 0, 0)
	root.Hub.Ports = []model.Port{socketPort(1, first)}
	topo := &model.Topology{Controllers: []model.Controller{{ID: "c1", RootHub: root}}}
	Annotate(topo)

	if encOf(t, topo, "second") == encOf(t, topo, "first") {
		t.Error("hubs merged on an unfilled port table, want one box each")
	}
}

func TestEnclosureNeverAbsorbsAHubThroughARootHubPort(t *testing.T) {
	// The computer's own port table is the one known to be wrong, and a
	// dock plugged into a port it mislabels as internal would vanish into
	// "this computer". A hub reached through a root hub therefore always
	// stays a box of its own, even when the firmware calls the port
	// internal. The cost is that a hub genuinely soldered to the board
	// draws separately, which is only cosmetic.
	onboard := hub("onboard", 0x2109, 0x0822)
	root := hub("root", 0, 0)
	root.Hub.Ports = []model.Port{internalPort(1, onboard), socketPort(2, nil)}
	topo := &model.Topology{Controllers: []model.Controller{{ID: "c1", RootHub: root}}}
	Annotate(topo)

	if got := encOf(t, topo, "onboard"); got != "hub:onboard" {
		t.Errorf("enclosure = %q, want hub:onboard", got)
	}
}

func TestEnclosureNeverAbsorbsAKnownDock(t *testing.T) {
	// A dock is a box someone plugged in, so it can never be wiring inside
	// another box however its uplink port is labelled. This is the shape
	// that made a whole CalDigit TS4 disappear into the computer.
	inner := hub("dock-inner", ts4Vendor, ts4USB3Mid)
	dock := hub("dock-top", ts4Vendor, ts4USB3Top, inner)
	feeder := &model.Device{ID: "feeder", VendorID: 0x2109, ProductID: 0x0822,
		Hub: &model.Hub{PortCount: 2, Ports: []model.Port{internalPort(1, dock)}}}
	root := hub("root", 0, 0)
	root.Hub.Ports = []model.Port{socketPort(1, feeder)}
	topo := &model.Topology{Controllers: []model.Controller{{ID: "c1", RootHub: root}}}
	Annotate(topo)

	got := encOf(t, topo, "dock-top")
	if got != "dock:caldigit-ts4:1" {
		t.Fatalf("dock enclosure = %q, want dock:caldigit-ts4:1", got)
	}
	if got == encOf(t, topo, "feeder") {
		t.Error("the dock was absorbed into the hub above it")
	}
}

// The regression itself: on a machine whose firmware marks its own ports
// as not user connectable, a dock plugged into one must still be a dock.
func TestEnclosureKeepsTheDockWhenEveryRootPortClaimsToBeInternal(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/fixtures/deviant-caldigit-ts4.json")
	if err != nil {
		t.Fatal(err)
	}
	var topo model.Topology
	if err := json.Unmarshal(raw, &topo); err != nil {
		t.Fatal(err)
	}
	for i := range topo.Controllers {
		root := topo.Controllers[i].RootHub
		if root == nil || root.Hub == nil {
			continue
		}
		for j := range root.Hub.Ports {
			root.Hub.Ports[j].Connector = &model.Connector{UserConnectable: false}
		}
	}
	Annotate(&topo)

	boxes := map[string]int{}
	topo.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		if d.Hub != nil && d.Enclosure != nil {
			boxes[d.Enclosure.ID]++
		}
	})
	if boxes["dock:caldigit-ts4:1"] != 8 {
		t.Errorf("boxes = %v, want the dock still holding its 8 hubs", boxes)
	}
	if boxes["host"] != 2 {
		t.Errorf("host holds %d hubs, want the 2 root hubs", boxes["host"])
	}
}
