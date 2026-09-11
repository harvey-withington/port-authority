package insight

import (
	"fmt"
	"strings"

	"portauthority/core/kb"
	"portauthority/core/model"
)

// routerName is the friendliest name for a USB4 router: the knowledge
// base's, else what the router's maker wrote into it, else the driver's.
func routerName(r *model.USB4Router) string {
	switch {
	case r.ProductName != "":
		return r.ProductName
	case r.Model != "":
		return strings.TrimSpace(r.Vendor + " " + r.Model)
	}
	return r.Name
}

// routerExpectation is the fastest link the knowledge base says the
// product behind a router can carry, and the product's name. A router
// nobody has recorded gets no expectation, so nothing is judged.
func routerExpectation(r *model.USB4Router) (string, model.LinkSpeed) {
	if known, ok := kb.KnownDeviceByUSB4ID(r.VendorID, r.ProductID); ok {
		return known.Name, known.MaxLink
	}
	if r.USBVendorID != 0 {
		if known, ok := kb.KnownDeviceInfo(r.USBVendorID, r.USBProductID); ok {
			return known.Name, known.MaxLink
		}
	}
	var dock *kb.Dock
	var ok bool
	if r.Model != "" {
		dock, ok = kb.DockForUSB4Strings(r.Vendor, r.Model)
	} else {
		dock, ok = kb.DockForUSB4Router(r.Name)
	}
	if ok {
		return dock.Name, dock.UplinkMaxLink
	}
	return "", model.LinkUnknown
}

// ruleUSB4LinkBelowMax flags a USB4 link that came up slower than the
// product on the far end can do. Everything behind a dock, its own
// Thunderbolt ports included, shares that one link, so a 20 Gbps cable on
// a 40 Gbps dock halves the whole desk.
func ruleUSB4LinkBelowMax(ctx *Context) []Insight {
	if ctx.Topology == nil {
		return nil
	}
	var out []Insight
	for i := range ctx.Topology.USB4 {
		r := &ctx.Topology.USB4[i]
		if r.Kind != "device" || r.NegotiatedLink == model.LinkUnknown {
			continue
		}
		name, max := routerExpectation(r)
		if max == model.LinkUnknown || r.NegotiatedLink >= max {
			continue
		}
		out = append(out, Insight{
			Severity: SeverityWarning,
			Title:    fmt.Sprintf("%s is connected at %s; it can do %s", name, speedWord(r.NegotiatedLink), speedWord(max)),
			Explanation: fmt.Sprintf("The USB4 / Thunderbolt link between %s and the computer came up at %s. Everything behind it shares that link: its own Thunderbolt ports, the USB tunnel every USB device in it uses, and any displays.",
				name, speedWord(r.NegotiatedLink)),
			Suggestion: "Check the cable first. Many USB-C cables, and passive cables longer than about 0.8 m, are limited to 20 Gbps. Use a cable marked Thunderbolt 4 or USB4 40 Gbps, and make sure it is in a Thunderbolt or USB4 port on the computer.",
			Evidence: []string{
				fmt.Sprintf("USB4 router %s negotiated Gen %d over %d lane(s): %s", r.InstanceID, r.LinkGen, r.LinkLanes, r.NegotiatedLink),
				fmt.Sprintf("knowledge base: %s can carry %s", name, max),
			},
			Confidence: 0.7,
			DeviceIDs:  []string{r.ID},
		})
	}
	return out
}

// ruleDockNotRecognised points at a chain of hubs that has the shape of a
// dock but is not in the knowledge base, so the user can tell Port
// Authority what it is and get it drawn as one box from then on.
//
// The shape: a hub that is not the computer's own and not in a known dock,
// with another such hub below it (docks are chains, single hubs are not),
// or a USB4 device router that no dock claimed and no known product
// explains. A dock reached over USB4 shows up as two chains, its USB 2 and
// USB 3 halves, so with one such router in play the chains are reported
// together as one dock.
func ruleDockNotRecognised(ctx *Context) []Insight {
	if ctx.Topology == nil {
		return nil
	}
	var untied []*model.USB4Router
	for i := range ctx.Topology.USB4 {
		r := &ctx.Topology.USB4[i]
		if r.Kind != "device" || r.EnclosureID != "" {
			continue
		}
		if _, ok := kb.KnownDeviceByUSB4ID(r.VendorID, r.ProductID); ok {
			continue
		}
		if _, ok := kb.KnownDeviceInfo(r.USBVendorID, r.USBProductID); ok && r.USBVendorID != 0 {
			continue
		}
		untied = append(untied, r)
	}

	// The tops of the unrecognised chains, with every unrecognised hub
	// below each of them.
	type chain struct {
		top     *Node
		members []*Node
	}
	var chains []chain
	for _, n := range ctx.Nodes {
		if !isLooseHub(n) || (n.Parent != nil && isLooseHub(n.Parent)) {
			continue
		}
		c := chain{top: n}
		for _, m := range ctx.Nodes {
			if isLooseHub(m) && (m == n || hasAncestor(m, n)) {
				c.members = append(c.members, m)
			}
		}
		chains = append(chains, c)
	}
	if len(chains) == 0 {
		return nil
	}

	build := func(name string, members []*Node, router *model.USB4Router) Insight {
		ids := make([]string, 0, len(members)+1)
		evidence := make([]string, 0, len(members)+1)
		for _, m := range members {
			ids = append(ids, m.Device.ID)
			evidence = append(evidence, fmt.Sprintf("hub %04x:%04x %s", m.Device.VendorID, m.Device.ProductID, m.Name()))
		}
		if router != nil {
			ids = append(ids, router.ID)
			evidence = append(evidence, fmt.Sprintf("USB4 router %s (%04x:%04x) belongs to no known dock", router.Name, router.VendorID, router.ProductID))
		}
		return Insight{
			Severity: SeverityInfo,
			Title:    fmt.Sprintf("%s looks like a dock Port Authority does not know yet", name),
			Explanation: "Its hubs are drawn as separate boxes because nothing says they belong together. " +
				"Windows reports a dock as a chain of hubs, often split across two controllers, and only the knowledge base can fold that back into one object.",
			Suggestion: "Tell Port Authority which hubs make up this dock and it will draw them as one dock from now on, on this machine.",
			Evidence:   evidence,
			Confidence: 0.6,
			DeviceIDs:  ids,
		}
	}

	// One untied router and it names the dock: every loose chain is a
	// half of it.
	if len(untied) == 1 {
		var members []*Node
		for _, c := range chains {
			members = append(members, c.members...)
		}
		return []Insight{build(routerName(untied[0]), members, untied[0])}
	}
	var out []Insight
	for _, c := range chains {
		if len(c.members) < 2 {
			continue
		}
		out = append(out, build(c.top.Name(), c.members, nil))
	}
	return out
}

// isLooseHub is a hub that belongs to no recognised box: not the computer's
// own root hub tree, not a known dock.
func isLooseHub(n *Node) bool {
	if !n.IsHub() || n.IsRoot() {
		return false
	}
	enc := n.Device.Enclosure
	return enc == nil || enc.Kind == model.EnclosureHub
}

func hasAncestor(n, ancestor *Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p == ancestor {
			return true
		}
	}
	return false
}
