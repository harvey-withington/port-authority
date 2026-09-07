// Package insight turns a Topology into plain-language findings. Rules are
// small predicates over an indexed view of the tree (Context); each emits
// zero or more Insights with a headline, an explanation, a concrete
// suggestion, the evidence behind it, and a confidence.
package insight

import (
	"fmt"
	"sort"

	"portauthority/core/kb"
	"portauthority/core/model"
)

// Severity orders insights for display.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

var severityRank = map[Severity]int{SeverityCritical: 0, SeverityWarning: 1, SeverityInfo: 2}

// Insight is one finding. Title, Explanation and Suggestion are written for
// people who do not know what SuperSpeedPlus means; Evidence is for those
// who do.
type Insight struct {
	RuleID      string   `json:"rule_id"`
	Severity    Severity `json:"severity"`
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Suggestion  string   `json:"suggestion"`
	Evidence    []string `json:"evidence,omitempty"`
	Confidence  float64  `json:"confidence"`
	DeviceIDs   []string `json:"device_ids,omitempty"`
}

// Rule is a named predicate over a Context.
type Rule struct {
	ID       string
	Evaluate func(*Context) []Insight
}

// Node is a device with its place in the tree.
type Node struct {
	Device     *model.Device
	Port       *model.Port // nil for root hubs
	Parent     *Node       // nil for root hubs
	Controller *model.Controller
}

// Context is the indexed topology rules run against.
type Context struct {
	Topology *model.Topology
	Nodes    []*Node // pre-order
	ByID     map[string]*Node
}

// NewContext indexes a topology.
func NewContext(t *model.Topology) *Context {
	ctx := &Context{Topology: t, ByID: map[string]*Node{}}
	byDevice := map[*model.Device]*Node{}
	t.Walk(func(c *model.Controller, parent *model.Device, port *model.Port, d *model.Device) {
		n := &Node{Device: d, Port: port, Controller: c}
		if parent != nil {
			n.Parent = byDevice[parent]
		}
		byDevice[d] = n
		ctx.Nodes = append(ctx.Nodes, n)
		ctx.ByID[d.ID] = n
	})
	return ctx
}

// IsHub reports whether the node has downstream ports.
func (n *Node) IsHub() bool { return n.Device.Hub != nil }

// IsRoot reports whether the node is a controller's root hub.
func (n *Node) IsRoot() bool { return n.Parent == nil }

// Name is the friendliest available name for the device.
func (n *Node) Name() string {
	d := n.Device
	if known, ok := kb.KnownDeviceInfo(d.VendorID, d.ProductID); ok {
		return known.Name
	}
	product := firstNonEmpty(d.Product, d.ProductName, d.FriendlyName, d.Description)
	vendor := firstNonEmpty(d.VendorName, d.Manufacturer)
	switch {
	case product == "" && vendor == "":
		return fmt.Sprintf("device %04x:%04x", d.VendorID, d.ProductID)
	case vendor == "":
		return product
	case product == "":
		return vendor + " device"
	}
	return vendor + " " + product
}

// Ancestors lists hub ancestors nearest-first, ending with the root hub.
func (n *Node) Ancestors() []*Node {
	var out []*Node
	for p := n.Parent; p != nil; p = p.Parent {
		out = append(out, p)
	}
	return out
}

// Dock finds the known dock the node is attached through. It returns the
// dock and the topmost node belonging to it (the dock's uplink hub).
func (n *Node) Dock() (*kb.Dock, *Node) {
	var dock *kb.Dock
	var top *Node
	for _, a := range n.Ancestors() {
		d := a.Device
		if dock == nil {
			if found, ok := kb.DockForHub(d.VendorID, d.ProductID); ok {
				dock, top = found, a
			}
			continue
		}
		if dock.HasHub(d.VendorID, d.ProductID) || kb.IsGenericDockHub(d.VendorID, d.ProductID) {
			top = a
			continue
		}
		break
	}
	return dock, top
}

// HopCount is the number of user-visible hubs between the node and the
// computer. A dock's internal hub chain counts as one hop.
func (n *Node) HopCount() int {
	hops := 0
	var current *kb.Dock
	for _, a := range n.Ancestors() {
		if a.IsRoot() {
			break
		}
		d := a.Device
		dock, known := kb.DockForHub(d.VendorID, d.ProductID)
		switch {
		case known && dock == current:
			// still inside the same dock
		case known:
			current = dock
			hops++
		case current != nil && kb.IsGenericDockHub(d.VendorID, d.ProductID):
			// dock's chipset hub, still the same dock
		default:
			current = nil
			hops++
		}
	}
	return hops
}

// Evaluate runs the default rule set.
func Evaluate(t *model.Topology) []Insight {
	return EvaluateWith(NewContext(t), DefaultRules())
}

// EvaluateWith runs the given rules and returns insights ordered by severity,
// then rule id, then title.
func EvaluateWith(ctx *Context, rules []Rule) []Insight {
	var out []Insight
	for _, r := range rules {
		for _, in := range r.Evaluate(ctx) {
			if in.RuleID == "" {
				in.RuleID = r.ID
			}
			out = append(out, in)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if severityRank[out[i].Severity] != severityRank[out[j].Severity] {
			return severityRank[out[i].Severity] < severityRank[out[j].Severity]
		}
		if out[i].RuleID != out[j].RuleID {
			return out[i].RuleID < out[j].RuleID
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
