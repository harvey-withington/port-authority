// Package enrich annotates a Topology with information that is not observed
// from the hardware, such as vendor and product names from the knowledge
// base. It only fills fields that are empty; provider-supplied values win.
package enrich

import (
	"portauthority/core/kb"
	"portauthority/core/model"
)

// Annotate fills Device.VendorName and Device.ProductName from the usb.ids
// knowledge base for every device with a non-zero ID. Existing non-empty
// values are left untouched.
func Annotate(t *model.Topology) {
	if t == nil {
		return
	}
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
	})
}

func isVague(c model.DeviceClass) bool {
	switch c {
	case model.ClassUnknown, model.ClassVendor, model.ClassComposite, "":
		return true
	}
	return false
}
