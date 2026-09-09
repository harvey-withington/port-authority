// Package enrich annotates a Topology with information that is not observed
// from the hardware, such as vendor and product names from the knowledge
// base. It only fills fields that are empty; provider-supplied values win.
package enrich

import (
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
		known, ok := kb.KnownDeviceByUSB4ID(r.VendorID, r.ProductID)
		if !ok {
			continue
		}
		if r.ProductName == "" {
			r.ProductName = known.Name
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
