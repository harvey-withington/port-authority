// Package enrich annotates a Topology with information that is not observed
// from the hardware, such as vendor and product names from the knowledge
// base. It only fills fields that are empty; provider-supplied values win.
package enrich

import (
	"strings"

	"portauthority/core/kb"
	"portauthority/core/model"
)

// Annotate fills Device.VendorName and Device.ProductName from the usb.ids
// knowledge base for every device with a non-zero ID, and groups the logical
// hubs into the physical boxes they live in. Existing non-empty values are
// left untouched.
func Annotate(t *model.Topology) {
	if t == nil {
		return
	}
	annotateEnclosures(t)
	// After the hubs have their boxes, and even when there are no hubs: a
	// router's tie is recomputed on every pass, not only when boxes exist.
	annotateRouterEnclosures(t)
	annotateRouters(t)
	t.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		if d.VendorID == 0 {
			return
		}
		if d.VendorName == "" {
			d.VendorName = kb.VendorName(d.VendorID)
		}
		if d.ProductID != 0 && d.ProductName == "" {
			d.ProductName = kb.ProductName(d.VendorID, d.ProductID)
		}
		// Descriptors cannot say "this vendor-class device is a display";
		// the knowledge base can. Only refine classes the descriptors left vague.
		if known, ok := kb.KnownDeviceInfo(d.VendorID, d.ProductID); ok && isVague(d.Class) {
			if refined := model.DeviceClass(known.Kind); refined != "" {
				d.Class = refined
			}
		}
		// A hub that belongs to a known dock can have its downstream ports
		// resolved to the dock's printed labels and positions.
		if d.Hub == nil {
			return
		}
		dock, ok := kb.DockForHub(d.VendorID, d.ProductID)
		if !ok {
			return
		}
		for i := range d.Hub.Ports {
			hp := &d.Hub.Ports[i]
			mapping, ok := dock.PhysicalPort(d.VendorID, d.ProductID, hp.Number)
			if !ok {
				continue
			}
			if hp.Label == "" {
				hp.Label = mapping.Label
			}
			if hp.Position == "" {
				hp.Position = mapping.Position
			}
		}
	})
}

// annotateRouters names the USB4 / Thunderbolt routers from the knowledge
// base. A router identifies itself by the bridge silicon inside it, so
// without this a USB4 SSD shows up as its controller chip rather than as
// the drive the person plugged in.
func annotateRouters(t *model.Topology) {
	for i := range t.USB4 {
		r := &t.USB4[i]
		if r.ProductName != "" {
			continue
		}
		known, ok := kb.KnownDeviceByUSB4ID(r.VendorID, r.ProductID)
		if !ok && r.USBVendorID != 0 {
			// The DROM carries the product's USB identity, so a device
			// listed by its USB id needs no separate router alias.
			known, ok = kb.KnownDeviceInfo(r.USBVendorID, r.USBProductID)
		}
		switch {
		case ok:
			r.ProductName = known.Name
		case r.Kind == "device" && r.Model != "":
			// What the maker wrote into the router, which beats the
			// driver's "USB4 Router (1.0), <vendor> - <model>".
			r.ProductName = strings.TrimSpace(r.Vendor + " " + r.Model)
		}
	}
}

func isVague(c model.DeviceClass) bool {
	switch c {
	case model.ClassUnknown, model.ClassVendor, model.ClassComposite, "":
		return true
	}
	return false
}
