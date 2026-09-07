package insight

import (
	"fmt"
	"strings"

	"portauthority/core/kb"
	"portauthority/core/model"
)

// DefaultRules is the shipped rule set, in evaluation order.
func DefaultRules() []Rule {
	return []Rule{
		{ID: "faster-port-available", Evaluate: ruleFasterPortAvailable},
		{ID: "link-downgrade", Evaluate: ruleLinkDowngrade},
		{ID: "usb2-on-dock", Evaluate: ruleUSB2OnDock},
		{ID: "hub-chain-depth", Evaluate: ruleHubChainDepth},
		{ID: "power-budget", Evaluate: rulePowerBudget},
		{ID: "iso-reservation-squeeze", Evaluate: ruleIsoReservationSqueeze},
		{ID: "usb4-device-well-placed", Evaluate: ruleUSB4DeviceWellPlaced},
	}
}

// speedWord renders a link for people: "10 Gbps" rather than "ss10".
func speedWord(l model.LinkSpeed) string {
	switch l {
	case model.LinkLow:
		return "USB 1 low speed"
	case model.LinkFull:
		return "USB 1 full speed (12 Mbps)"
	case model.LinkHigh:
		return "USB 2 speed (480 Mbps)"
	case model.LinkSuper:
		return "5 Gbps"
	case model.LinkSuperPlus:
		return "10 Gbps"
	case model.LinkSuperPlus2x2:
		return "20 Gbps"
	case model.LinkUSB4Gen2:
		return "20 Gbps (USB4)"
	case model.LinkUSB4Gen3:
		return "40 Gbps (USB4 / Thunderbolt)"
	case model.LinkUSB4Gen4:
		return "80 Gbps (USB4 v2)"
	}
	return l.String()
}

func speedRatio(fast, slow model.LinkSpeed) string {
	if slow.Bitrate() == 0 {
		return ""
	}
	r := float64(fast.Bitrate()) / float64(slow.Bitrate())
	if r < 1.5 {
		return ""
	}
	return fmt.Sprintf("up to %.0fx", r)
}

func portLabel(dock *kb.Dock, hub *Node, port *model.Port) (string, bool) {
	if dock == nil || hub == nil || port == nil {
		return "", false
	}
	m, ok := dock.PhysicalPort(hub.Device.VendorID, hub.Device.ProductID, port.Number)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("the %q port on the %s", m.Label, m.Position), true
}

// describePorts renders port kinds the way a person would point at them:
// `one of the 2 "Thunderbolt 4" ports on the rear`.
func describePorts(ports []kb.DockPort) string {
	var parts []string
	for _, p := range ports {
		var s string
		switch {
		case p.Count > 1:
			s = fmt.Sprintf("one of the %d %q ports", p.Count, p.Label)
		default:
			s = fmt.Sprintf("the %q port", p.Label)
		}
		if p.Position != "" {
			s += " on the " + p.Position
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", or ")
}

// ruleFasterPortAvailable: a device the knowledge base knows can go faster
// than the port it is on, with a concrete better port to move it to.
func ruleFasterPortAvailable(ctx *Context) []Insight {
	var out []Insight
	for _, n := range ctx.Nodes {
		if n.IsHub() || n.Port == nil || n.Port.Status != model.StatusConnected {
			continue
		}
		known, ok := kb.KnownDeviceInfo(n.Device.VendorID, n.Device.ProductID)
		if !ok || known.MaxLink <= n.Port.MaxLink {
			continue
		}
		name := n.Name()
		ratio := speedRatio(known.MaxLink, n.Port.NegotiatedLink)
		in := Insight{
			Severity:  SeverityWarning,
			DeviceIDs: []string{n.Device.ID},
			Evidence: []string{
				fmt.Sprintf("negotiated link %s, port maximum %s", n.Port.NegotiatedLink, n.Port.MaxLink),
				fmt.Sprintf("knowledge base: %s supports %s", known.Name, known.MaxLink),
				fmt.Sprintf("port path %s", n.Device.PortPath),
			},
		}
		if ratio != "" {
			in.Title = fmt.Sprintf("%s could be %s faster on a different port", name, ratio)
		} else {
			in.Title = fmt.Sprintf("%s could be faster on a different port", name)
		}
		dock, top := n.Dock()
		where := fmt.Sprintf("a %s port", speedWord(n.Port.MaxLink))
		if dock != nil {
			if label, ok := portLabel(dock, n.Parent, n.Port); ok {
				where = fmt.Sprintf("%s of the %s, which is a %s port", label, dock.Name, speedWord(n.Port.MaxLink))
				in.Confidence = 0.9
			} else {
				where = fmt.Sprintf("one of the %s's %s ports", dock.Name, speedWord(n.Port.MaxLink))
				in.Confidence = 0.8
			}
			in.Explanation = fmt.Sprintf("%s supports %s, but it is plugged into %s. Everything it does is capped by that port.",
				name, speedWord(known.MaxLink), where)
			better := dock.PortsAtLeast(known.MaxLink)
			if len(better) == 0 {
				better = dock.PortsAtLeast(n.Port.MaxLink + 1)
			}
			if len(better) > 0 {
				in.Suggestion = fmt.Sprintf("Move %s to %s of the %s.", name, describePorts(better), dock.Name)
				for _, p := range better {
					if p.Notes != "" {
						in.Evidence = append(in.Evidence, fmt.Sprintf("%s: %s", p.Label, p.Notes))
					}
				}
			} else {
				in.Suggestion = fmt.Sprintf("The %s has no faster port. Plug %s directly into a %s port on the computer, if it has one.",
					dock.Name, name, speedWord(known.MaxLink))
				in.Confidence = 0.7
			}
			if top != nil {
				in.Evidence = append(in.Evidence, fmt.Sprintf("dock uplink hub %s", top.Device.ID))
			}
		} else {
			in.Confidence = 0.6
			in.Explanation = fmt.Sprintf("%s supports %s, but it is plugged into %s. Everything it does is capped by that port.",
				name, speedWord(known.MaxLink), where)
			in.Suggestion = fmt.Sprintf("Plug %s into a %s port, if the computer or dock has one.", name, speedWord(known.MaxLink))
		}
		out = append(out, in)
	}
	return out
}

// ruleLinkDowngrade: negotiated below both what the device and the port can do.
func ruleLinkDowngrade(ctx *Context) []Insight {
	var out []Insight
	for _, n := range ctx.Nodes {
		p := n.Port
		if p == nil || p.Status != model.StatusConnected {
			continue
		}
		link, claimed, portMax := p.NegotiatedLink, n.Device.ClaimedSpeed, p.MaxLink
		if link == model.LinkUnknown || claimed == model.LinkUnknown || portMax == model.LinkUnknown {
			continue
		}
		if link >= claimed || link >= portMax {
			continue
		}
		name := n.Name()
		out = append(out, Insight{
			Severity: SeverityWarning,
			Title:    fmt.Sprintf("%s is running slower than it should", name),
			Explanation: fmt.Sprintf("%s connected at %s, but the device can do %s and the port can do %s. This usually means a cable that does not support the higher speed, a loose connection, or a device that fell back after an error.",
				name, speedWord(link), speedWord(claimed), speedWord(portMax)),
			Suggestion: "Unplug it and plug it back in. If that does not help, try a different cable, then a different port.",
			Evidence: []string{
				fmt.Sprintf("negotiated %s, device claims %s, port supports %s", link, claimed, portMax),
				fmt.Sprintf("port path %s", n.Device.PortPath),
			},
			Confidence: 0.8,
			DeviceIDs:  []string{n.Device.ID},
		})
	}
	return out
}

// ruleUSB2OnDock: a USB 3 capable device stuck on the USB 2 side of a dock.
func ruleUSB2OnDock(ctx *Context) []Insight {
	var out []Insight
	for _, n := range ctx.Nodes {
		p := n.Port
		if n.IsHub() || p == nil || p.Status != model.StatusConnected {
			continue
		}
		if n.Device.ClaimedSpeed < model.LinkSuper || p.NegotiatedLink > model.LinkHigh {
			continue
		}
		dock, _ := n.Dock()
		if dock == nil {
			continue
		}
		name := n.Name()
		out = append(out, Insight{
			Severity: SeverityWarning,
			Title:    fmt.Sprintf("%s is stuck at USB 2 speed on the %s", name, dock.Name),
			Explanation: fmt.Sprintf("%s can do %s but is connected at %s. On docks this often happens when a display uses the USB-C lanes (DisplayPort alt mode leaves only USB 2 for data), or when the device's USB 3 link failed and it fell back.",
				name, speedWord(n.Device.ClaimedSpeed), speedWord(p.NegotiatedLink)),
			Suggestion: fmt.Sprintf("Try a different port on the %s, or a port on the computer itself. If a display is plugged into the dock's USB-C port, that can be the cause.", dock.Name),
			Evidence: []string{
				fmt.Sprintf("negotiated %s, device claims %s", p.NegotiatedLink, n.Device.ClaimedSpeed),
				fmt.Sprintf("on the dock's USB 2 hub %04x:%04x, port path %s", n.Parent.Device.VendorID, n.Parent.Device.ProductID, n.Device.PortPath),
			},
			Confidence: 0.6,
			DeviceIDs:  []string{n.Device.ID},
		})
	}
	return out
}

// ruleHubChainDepth: too many user-visible hubs between device and computer.
func ruleHubChainDepth(ctx *Context) []Insight {
	const limit = 3
	var out []Insight
	for _, n := range ctx.Nodes {
		if n.IsHub() || n.Port == nil || n.Port.Status != model.StatusConnected {
			continue
		}
		hops := n.HopCount()
		if hops < limit {
			continue
		}
		name := n.Name()
		out = append(out, Insight{
			Severity: SeverityInfo,
			Title:    fmt.Sprintf("%s is behind %d chained hubs", name, hops),
			Explanation: fmt.Sprintf("Every hub adds latency and shares its uplink with everything below it. USB allows five tiers, but connections get flaky well before that. %s is %d hubs away from the computer.",
				name, hops),
			Suggestion: fmt.Sprintf("Plug %s, or the hub it is on, closer to the computer.", name),
			Evidence:   []string{fmt.Sprintf("port path %s, %d user-visible hops (dock internals count as one)", n.Device.PortPath, hops)},
			Confidence: 0.7,
			DeviceIDs:  []string{n.Device.ID},
		})
	}
	return out
}

// rulePowerBudget: a bus-powered hub whose devices ask for more than it has.
func rulePowerBudget(ctx *Context) []Insight {
	var out []Insight
	for _, n := range ctx.Nodes {
		hub := n.Device.Hub
		if hub == nil || !hub.BusPowered || n.IsRoot() {
			continue
		}
		budget := 500
		if n.Port != nil && n.Port.NegotiatedLink >= model.LinkSuper {
			budget = 900
		}
		total := 0
		var ids, parts []string
		for i := range hub.Ports {
			d := hub.Ports[i].Device
			if d == nil || d.PowerDrawMA == 0 {
				continue
			}
			total += d.PowerDrawMA
			ids = append(ids, d.ID)
			parts = append(parts, fmt.Sprintf("%s %d mA", (&Node{Device: d}).Name(), d.PowerDrawMA))
		}
		if total <= budget {
			continue
		}
		name := n.Name()
		out = append(out, Insight{
			Severity: SeverityWarning,
			Title:    fmt.Sprintf("%s may not have enough power for what is plugged into it", name),
			Explanation: fmt.Sprintf("%s takes its power from the computer and can share about %d mA. The devices on it ask for %d mA together. Overloaded hubs drop devices at random or fail to spin up drives.",
				name, budget, total),
			Suggestion: "Use a hub with its own power supply, or spread the power-hungry devices across different ports.",
			Evidence:   append([]string{fmt.Sprintf("bus-powered hub, budget %d mA, requested %d mA", budget, total)}, parts...),
			Confidence: 0.7,
			DeviceIDs:  append([]string{n.Device.ID}, ids...),
		})
	}
	return out
}

// ruleIsoReservationSqueeze: streaming devices reserving a big share of a
// link that storage on the same controller needs.
func ruleIsoReservationSqueeze(ctx *Context) []Insight {
	type acc struct {
		iso     model.Bitrate
		isoDevs []*Node
		storage []*Node
	}
	perController := map[*model.Controller]*acc{}
	for _, n := range ctx.Nodes {
		if n.IsHub() || n.Port == nil || n.Port.Status != model.StatusConnected {
			continue
		}
		a := perController[n.Controller]
		if a == nil {
			a = &acc{}
			perController[n.Controller] = a
		}
		if n.Device.IsoReserved > 0 {
			a.iso += n.Device.IsoReserved
			a.isoDevs = append(a.isoDevs, n)
		}
		if n.Device.Class == model.ClassStorage {
			a.storage = append(a.storage, n)
		}
	}
	var out []Insight
	for c, a := range perController {
		if a.iso == 0 || len(a.storage) == 0 {
			continue
		}
		for _, s := range a.storage {
			capacity := s.Port.NegotiatedLink.Bitrate()
			if capacity == 0 || a.iso*5 < capacity { // fire at 20% or more
				continue
			}
			var names, ids []string
			for _, d := range a.isoDevs {
				names = append(names, d.Name())
				ids = append(ids, d.Device.ID)
			}
			name := s.Name()
			out = append(out, Insight{
				Severity: SeverityWarning,
				Title:    fmt.Sprintf("%s shares its controller with streaming devices that reserve %s", name, a.iso),
				Explanation: fmt.Sprintf("%s reserve %s of guaranteed bandwidth on %s for audio or video. That reservation comes off the top before %s gets any.",
					strings.Join(names, ", "), a.iso, c.Name, name),
				Suggestion: fmt.Sprintf("Move %s, or the streaming devices, to a port on a different controller.", name),
				Evidence:   []string{fmt.Sprintf("isochronous reservation %s vs storage link %s", a.iso, capacity)},
				Confidence: 0.6,
				DeviceIDs:  append([]string{s.Device.ID}, ids...),
			})
		}
	}
	return out
}

// ruleUSB4DeviceWellPlaced: a known 40 Gbps device that shows up as a USB4
// device router is tunneling PCIe, which only happens on a USB4 /
// Thunderbolt port. It has left the USB hub tree, so this is the one
// place to confirm it is where it should be. Context only indexes USB
// devices; the routers are read straight from the topology.
func ruleUSB4DeviceWellPlaced(ctx *Context) []Insight {
	if ctx.Topology == nil {
		return nil
	}
	var out []Insight
	for i := range ctx.Topology.USB4 {
		r := &ctx.Topology.USB4[i]
		if r.Kind != "device" {
			continue
		}
		known, ok := kb.KnownDeviceByUSB4ID(r.VendorID, r.ProductID)
		if !ok || known.MaxLink < model.LinkUSB4Gen3 {
			continue
		}
		name := known.Name
		evidence := []string{
			fmt.Sprintf("USB4 device router %s (usb4 id %04x:%04x, depth %d)", r.InstanceID, r.VendorID, r.ProductID, r.Depth),
			fmt.Sprintf("knowledge base: %s supports %s", name, known.MaxLink),
		}
		for _, c := range r.Children {
			evidence = append(evidence, "carries "+c)
		}
		out = append(out, Insight{
			Severity: SeverityInfo,
			Title:    fmt.Sprintf("%s is on a %s port", name, speedWord(model.LinkUSB4Gen3)),
			Explanation: fmt.Sprintf("%s is connected as a USB4 router, which means it tunnels PCIe instead of running as a USB drive. It shows up as an NVMe disk and runs at full speed; that is also why it is no longer listed under any USB hub.",
				name),
			Suggestion: "Nothing to do.",
			Evidence:   evidence,
			Confidence: 0.8,
			DeviceIDs:  []string{r.ID},
		})
	}
	return out
}
