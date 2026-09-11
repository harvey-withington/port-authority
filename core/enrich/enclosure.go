package enrich

import (
	"fmt"
	"strings"

	"portauthority/core/kb"
	"portauthority/core/model"
)

// annotateEnclosures groups the logical hubs the OS reports into the
// physical boxes a person can see.
//
// Windows reports a dock as a chain of four or five hubs across two
// controllers (its USB 2 half and its USB 3 half arrive separately), and
// reports the computer's own internals as several controllers each with a
// root hub. Neither matches what is on the desk: one laptop, one dock. This
// pass labels every hub with the box it belongs to, so the diagram can draw
// boxes and cables instead of the chain.
//
// The rules are deliberately conservative. Membership comes from the dock
// knowledge base, never from a guess about hub nesting, so an unrecognised
// hub is its own box rather than being folded into whatever sits above it.
func annotateEnclosures(t *model.Topology) {
	hubs := collectHubs(t)
	if len(hubs) == 0 {
		return
	}
	sets := groupHubs(hubs, indexHubPaths(t))
	assignEnclosures(hubs, sets)
}

// dockForRouter recognises a dock by the strings its router announces:
// the DROM vendor and model when the provider read them, else the ones
// Windows folds into the router's name.
func dockForRouter(r *model.USB4Router) (*kb.Dock, bool) {
	if r.Model != "" {
		return kb.DockForUSB4Strings(r.Vendor, r.Model)
	}
	return kb.DockForUSB4Router(r.Name)
}

// annotateRouterEnclosures ties a dock's USB4 router to the box its hubs
// were grouped into, so the diagram can draw one dock with one cable and
// hang what the router carries off the dock rather than off the computer.
//
// The router is recognised by the vendor and model strings it announces,
// which the knowledge base records per dock. With two docks of one model
// attached there is nothing to say which router is which, so neither is
// tied to a box: the routers stay separate rather than being guessed.
func annotateRouterEnclosures(t *model.Topology) {
	if len(t.USB4) == 0 {
		return
	}
	boxesByDock := map[string]map[string]bool{}
	t.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		enc := d.Enclosure
		if enc == nil || enc.Kind != model.EnclosureDock || enc.DockID == "" {
			return
		}
		if boxesByDock[enc.DockID] == nil {
			boxesByDock[enc.DockID] = map[string]bool{}
		}
		boxesByDock[enc.DockID][enc.ID] = true
	})
	for i := range t.USB4 {
		r := &t.USB4[i]
		// Derived, never observed: a replayed snapshot carries the tie its
		// knowledge base made, which this one must be free to undo.
		r.EnclosureID = ""
		if r.Kind != "device" {
			continue
		}
		dock, ok := dockForRouter(r)
		if !ok {
			continue
		}
		boxes := boxesByDock[dock.ID]
		if len(boxes) != 1 {
			continue
		}
		for id := range boxes {
			r.EnclosureID = id
		}
	}
}

// hubNode is one logical hub together with the hub it hangs off.
type hubNode struct {
	dev *model.Device
	// parent is the hub above this one; nil for a root hub.
	parent *model.Device
	// port is the parent's port this hub is plugged into, whose connector
	// says which other port shares the same physical socket. Nil for a
	// root hub.
	port *model.Port
	// dock is the knowledge-base dock this hub belongs to, if any.
	dock *kb.Dock
	// generic marks a chipset hub found inside many docks, which belongs
	// to whichever dock it sits next to.
	generic bool
}

func (n *hubNode) isRoot() bool { return n.parent == nil }

// productKey packs a VID and PID into one comparable value.
func productKey(d *model.Device) uint32 {
	return uint32(d.VendorID)<<16 | uint32(d.ProductID)
}

func collectHubs(t *model.Topology) []*hubNode {
	var hubs []*hubNode
	t.Walk(func(_ *model.Controller, parent *model.Device, port *model.Port, d *model.Device) {
		if d.Hub == nil {
			return
		}
		n := &hubNode{dev: d, parent: parent, port: port}
		// A root hub is the computer's own; dock lookups do not apply.
		if parent != nil {
			if dock, ok := kb.DockForHub(d.VendorID, d.ProductID); ok {
				n.dock = dock
			}
			n.generic = kb.IsGenericDockHub(d.VendorID, d.ProductID)
		}
		hubs = append(hubs, n)
	})
	return hubs
}

// normalizeHubPath makes two spellings of one device path comparable: some
// sources carry a "\?\" or "\.\" namespace prefix and others do not.
func normalizeHubPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if strings.HasPrefix(trimmed, `\\?\`) || strings.HasPrefix(trimmed, `\\.\`) {
		trimmed = trimmed[4:]
	}
	return strings.ToUpper(trimmed)
}

// indexHubPaths maps every hub's device path to the hub itself, so a
// connector's companion path resolves in one lookup.
func indexHubPaths(t *model.Topology) map[string]*model.Device {
	index := map[string]*model.Device{}
	t.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		if d.Hub != nil && d.Hub.DevicePath != "" {
			index[normalizeHubPath(d.Hub.DevicePath)] = d
		}
	})
	return index
}

// companionHub returns the hub sharing this hub's physical socket.
//
// Windows reports one logical port per USB 2 / USB 3 half of a socket and
// links the pair through the connector descriptor. Two devices cannot be
// plugged into one socket, so when both halves hold a hub they are the two
// halves of a single hub: the same object, enumerated twice. This is the
// one place a box can be recognised without the knowledge base, because
// the OS is stating it rather than the shape of the tree implying it.
func companionHub(n *hubNode, byPath map[string]*model.Device) *model.Device {
	if n.port == nil || n.port.Connector == nil {
		return nil
	}
	c := n.port.Connector
	// Port numbers are 1-based, so a zero companion means "not reported".
	if c.CompanionHubPath == "" || c.CompanionPort == 0 {
		return nil
	}
	peer, ok := byPath[normalizeHubPath(c.CompanionHubPath)]
	if !ok || peer.Hub == nil {
		return nil
	}
	for i := range peer.Hub.Ports {
		p := &peer.Hub.Ports[i]
		if p.Number != c.CompanionPort {
			continue
		}
		if p.Device != nil && p.Device.Hub != nil {
			return p.Device
		}
		return nil
	}
	return nil
}

// insideParent reports whether a hub hangs off a port the firmware says is
// not a socket, which makes it wiring inside the parent's box rather than
// something plugged into it.
//
// This acts on a positive assertion from the OS, never on the shape of the
// tree: hubs nest for all sorts of reasons and only the firmware knows
// which of its ports reach the outside. Cheap hubs leave the table unfilled
// and mark everything connectable, so the rule simply does not fire for
// them and they stay as separate boxes, which is the honest answer when
// nothing can distinguish an internal chip from a plugged-in hub.
//
// Two things are never absorbed, because being wrong about them hides a
// whole object rather than merging two chips inside one:
//
//   - anything on a root hub. The computer's own port table is the one we
//     have evidence is unreliable (see plan/TODO.md on DEVIANT's phantom
//     sockets), and a dock plugged into a port it mislabels would
//     disappear into "this computer".
//   - a known dock, which is by definition a box someone plugged in and so
//     can never be wiring inside another one.
//
// The cost of these exceptions is that a hub genuinely soldered inside the
// chassis draws as its own box. That is a cosmetic imperfection; a
// vanishing dock is a broken picture.
func insideParent(child, parent *hubNode) bool {
	if parent.isRoot() || child.dock != nil {
		return false
	}
	return child.port != nil && child.port.Connector != nil && !child.port.Connector.UserConnectable
}

// sameBox reports whether a hub and the hub above it are inside one box.
func sameBox(child, parent *hubNode) bool {
	switch {
	// A hub wired to a port that is not a socket at all.
	case insideParent(child, parent):
		return true
	// Two hubs of the same known dock.
	case child.dock != nil && child.dock == parent.dock:
		return true
	// The dock's chipset hub, sitting above the hub that identifies it.
	case child.dock != nil && parent.generic && parent.dock == nil && !parent.isRoot():
		return true
	// The dock's chipset hub, sitting below the hub that identifies it.
	case child.generic && parent.dock != nil:
		return true
	// A run of chipset hubs, so the whole run joins whichever dock the
	// bottom of it identifies.
	case child.generic && parent.generic && !parent.isRoot():
		return true
	}
	return false
}

// groupHubs partitions the hubs into boxes, returning each box as a list of
// indexes into hubs, boxes in the order their first hub was walked.
func groupHubs(hubs []*hubNode, byPath map[string]*model.Device) [][]int {
	index := make(map[*model.Device]int, len(hubs))
	for i, n := range hubs {
		index[n.dev] = i
	}

	parent := make([]int, len(hubs))
	for i, n := range hubs {
		parent[i] = -1
		if n.parent != nil {
			if p, ok := index[n.parent]; ok {
				parent[i] = p
			}
		}
	}

	// Union-find over "these two hubs are the same box", with the lowest
	// index as the representative so the result does not depend on the
	// order the unions happen in.
	uf := make([]int, len(hubs))
	for i := range uf {
		uf[i] = i
	}
	find := func(i int) int {
		for uf[i] != i {
			uf[i] = uf[uf[i]]
			i = uf[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			uf[rb] = ra
		} else {
			uf[ra] = rb
		}
	}

	// Every root hub is the computer.
	host := -1
	for i, n := range hubs {
		if !n.isRoot() {
			continue
		}
		if host < 0 {
			host = i
		} else {
			union(host, i)
		}
	}

	// The two halves of one hub, which the OS reports as separate hubs on
	// separate controllers. Done before the dock rules so that a dock whose
	// halves are companions folds even when it is not in the knowledge base.
	for i, n := range hubs {
		peer := companionHub(n, byPath)
		if peer == nil {
			continue
		}
		if j, ok := index[peer]; ok {
			union(i, j)
		}
	}

	// A chipset hub only joins a dock once the hub below it has, so repeat
	// until nothing moves rather than relying on walk order.
	for changed := true; changed; {
		changed = false
		for i, n := range hubs {
			p := parent[i]
			if p < 0 || find(p) == find(i) || !sameBox(n, hubs[p]) {
				continue
			}
			union(p, i)
			changed = true
		}
	}

	var order []int
	members := map[int][]int{}
	for i := range hubs {
		r := find(i)
		if _, seen := members[r]; !seen {
			order = append(order, r)
		}
		members[r] = append(members[r], i)
	}
	out := make([][]int, 0, len(order))
	for _, r := range order {
		out = append(out, members[r])
	}
	return out
}

// dockInstances hands out one enclosure ID per physical dock.
//
// A dock's USB 2 and USB 3 halves reach the computer through different
// controllers and so form two separate chains; they are one box and get one
// ID. Two docks of the same model must not merge, so chains only join while
// their hubs stay distinct: one dock exposes each of its logical hubs
// exactly once, and a repeated VID:PID therefore means a second dock.
type dockInstances struct {
	seen map[*kb.Dock][]map[uint32]bool
}

func (di *dockInstances) idFor(dock *kb.Dock, keys map[uint32]bool) string {
	if di.seen == nil {
		di.seen = map[*kb.Dock][]map[uint32]bool{}
	}
	for i, taken := range di.seen[dock] {
		if overlaps(taken, keys) {
			continue
		}
		for k := range keys {
			taken[k] = true
		}
		return fmt.Sprintf("dock:%s:%d", dock.ID, i+1)
	}
	fresh := map[uint32]bool{}
	for k := range keys {
		fresh[k] = true
	}
	di.seen[dock] = append(di.seen[dock], fresh)
	return fmt.Sprintf("dock:%s:%d", dock.ID, len(di.seen[dock]))
}

func overlaps(a, b map[uint32]bool) bool {
	for k := range b {
		if a[k] {
			return true
		}
	}
	return false
}

func assignEnclosures(hubs []*hubNode, sets [][]int) {
	var instances dockInstances
	// One Enclosure value per ID, so the two halves of a dock are the same
	// box and not two boxes that happen to agree.
	byID := map[string]*model.Enclosure{}

	for _, set := range sets {
		var dock *kb.Dock
		isHost := false
		for _, i := range set {
			if hubs[i].isRoot() {
				isHost = true
			}
			if dock == nil {
				dock = hubs[i].dock
			}
		}

		var enc *model.Enclosure
		switch {
		case isHost:
			enc = &model.Enclosure{ID: "host", Kind: model.EnclosureHost}
		case dock != nil:
			keys := map[uint32]bool{}
			for _, i := range set {
				keys[productKey(hubs[i].dev)] = true
			}
			id := instances.idFor(dock, keys)
			enc = &model.Enclosure{ID: id, Kind: model.EnclosureDock, Name: dock.Name, DockID: dock.ID, Source: string(dock.Source)}
		default:
			enc = &model.Enclosure{ID: "hub:" + hubs[set[0]].dev.ID, Kind: model.EnclosureHub}
		}
		if existing, ok := byID[enc.ID]; ok {
			enc = existing
		} else {
			byID[enc.ID] = enc
		}

		// Unlike the name fields, an enclosure is derived rather than
		// observed: no provider produces one. Replaying a snapshot that
		// already carries groupings has to recompute them, or a fixed
		// grouping rule could never reach an old capture.
		for _, i := range set {
			hubs[i].dev.Enclosure = enc
		}
	}
}
