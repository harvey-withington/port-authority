package kb

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"portauthority/core/model"
)

//go:embed data/docks.json
var docksRaw []byte

//go:embed data/devices.json
var devicesRaw []byte

// DockPort describes one kind of physical port on a dock.
type DockPort struct {
	Label     string          `json:"label"`
	Position  string          `json:"position"`
	Connector string          `json:"connector"`
	MaxLink   model.LinkSpeed `json:"max_link"`
	Count     int             `json:"count"`
	Notes     string          `json:"notes,omitempty"`
}

// PortMapping pins a logical hub port to a physical, labelled port.
type PortMapping struct {
	HubVendorID  uint16
	HubProductID uint16
	Port         int
	Label        string
	Position     string
	Verified     string
}

// Dock is a known dock: the logical hubs it exposes and its physical ports.
type Dock struct {
	ID            string
	Name          string
	UplinkKind    string
	UplinkMaxLink model.LinkSpeed
	Ports         []DockPort
	PortMap       []PortMapping
	hubs          map[uint32]bool
}

// KnownDevice is capability data a device does not report over USB.
type KnownDevice struct {
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	MaxLink          model.LinkSpeed `json:"max_link"`
	ClaimedReadMBps  int             `json:"claimed_read_mbps"`
	ClaimedWriteMBps int             `json:"claimed_write_mbps"`
	Notes            string          `json:"notes,omitempty"`
	// USB4ID is the router "vid:pid" the device presents when it tunnels
	// PCIe over USB4 / Thunderbolt, a different namespace from its USB
	// identity. Optional.
	USB4ID string `json:"usb4_id,omitempty"`
}

var (
	docksOnce      sync.Once
	docks          []*Dock
	dockByHub      map[uint32]*Dock
	genericDockHub map[uint32]string
	knownDevices   map[uint32]*KnownDevice
	knownByUSB4    map[uint32]*KnownDevice
)

// DockForHub returns the dock a logical hub belongs to.
func DockForHub(vid, pid uint16) (*Dock, bool) {
	docksOnce.Do(loadDocks)
	d, ok := dockByHub[productKey(vid, pid)]
	return d, ok
}

// IsGenericDockHub reports whether a hub is a chipset hub found inside many
// docks (it belongs to whichever dock sits below it).
func IsGenericDockHub(vid, pid uint16) bool {
	docksOnce.Do(loadDocks)
	_, ok := genericDockHub[productKey(vid, pid)]
	return ok
}

// KnownDeviceInfo returns curated capability data for a device.
func KnownDeviceInfo(vid, pid uint16) (*KnownDevice, bool) {
	docksOnce.Do(loadDocks)
	d, ok := knownDevices[productKey(vid, pid)]
	return d, ok
}

// KnownDeviceByUSB4ID resolves a device by its USB4 router vendor and
// product id (the "usb4_id" alias in devices.json).
func KnownDeviceByUSB4ID(vid, pid uint16) (*KnownDevice, bool) {
	docksOnce.Do(loadDocks)
	d, ok := knownByUSB4[productKey(vid, pid)]
	return d, ok
}

// HasHub reports whether the given logical hub is part of this dock.
func (d *Dock) HasHub(vid, pid uint16) bool {
	return d.hubs[productKey(vid, pid)]
}

// BestLink is the fastest link any of the dock's ports can offer.
func (d *Dock) BestLink() model.LinkSpeed {
	best := model.LinkUnknown
	for _, p := range d.Ports {
		if p.MaxLink > best {
			best = p.MaxLink
		}
	}
	return best
}

// PortsAtLeast lists port kinds that can carry the given link or better.
func (d *Dock) PortsAtLeast(link model.LinkSpeed) []DockPort {
	var out []DockPort
	for _, p := range d.Ports {
		if p.MaxLink >= link {
			out = append(out, p)
		}
	}
	return out
}

// PhysicalPort resolves a logical hub port to its printed label, when known.
func (d *Dock) PhysicalPort(hubVID, hubPID uint16, port int) (PortMapping, bool) {
	for _, m := range d.PortMap {
		if m.HubVendorID == hubVID && m.HubProductID == hubPID && m.Port == port {
			return m, true
		}
	}
	return PortMapping{}, false
}

type rawDocks struct {
	GenericInternalHubs map[string]string `json:"generic_internal_hubs"`
	Docks               []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Hubs   []string `json:"hubs"`
		Uplink struct {
			Kind    string          `json:"kind"`
			MaxLink model.LinkSpeed `json:"max_link"`
		} `json:"uplink"`
		Ports   []DockPort `json:"ports"`
		PortMap []struct {
			Hub      string `json:"hub"`
			Port     int    `json:"port"`
			Label    string `json:"label"`
			Position string `json:"position"`
			Verified string `json:"verified"`
		} `json:"port_map"`
	} `json:"docks"`
}

func loadDocks() {
	var raw rawDocks
	if err := json.Unmarshal(docksRaw, &raw); err != nil {
		panic(fmt.Sprintf("kb: docks.json: %v", err))
	}
	dockByHub = map[uint32]*Dock{}
	genericDockHub = map[uint32]string{}
	for k, desc := range raw.GenericInternalHubs {
		genericDockHub[mustParsePair(k, "docks.json generic_internal_hubs")] = desc
	}
	for _, rd := range raw.Docks {
		d := &Dock{
			ID: rd.ID, Name: rd.Name,
			UplinkKind: rd.Uplink.Kind, UplinkMaxLink: rd.Uplink.MaxLink,
			Ports: rd.Ports,
			hubs:  map[uint32]bool{},
		}
		for _, h := range rd.Hubs {
			key := mustParsePair(h, "docks.json hubs")
			d.hubs[key] = true
			dockByHub[key] = d
		}
		for _, m := range rd.PortMap {
			key := mustParsePair(m.Hub, "docks.json port_map")
			d.PortMap = append(d.PortMap, PortMapping{
				HubVendorID: uint16(key >> 16), HubProductID: uint16(key),
				Port: m.Port, Label: m.Label, Position: m.Position, Verified: m.Verified,
			})
		}
		docks = append(docks, d)
	}

	var rawDev struct {
		Devices map[string]*KnownDevice `json:"devices"`
	}
	if err := json.Unmarshal(devicesRaw, &rawDev); err != nil {
		panic(fmt.Sprintf("kb: devices.json: %v", err))
	}
	knownDevices = map[uint32]*KnownDevice{}
	knownByUSB4 = map[uint32]*KnownDevice{}
	for k, dev := range rawDev.Devices {
		knownDevices[mustParsePair(k, "devices.json")] = dev
		if dev.USB4ID != "" {
			knownByUSB4[mustParsePair(dev.USB4ID, "devices.json usb4_id")] = dev
		}
	}
}

// mustParsePair parses "vid:pid" hex into a product key.
func mustParsePair(s, where string) uint32 {
	vidStr, pidStr, ok := strings.Cut(s, ":")
	if !ok {
		panic(fmt.Sprintf("kb: %s: key %q must be vid:pid", where, s))
	}
	vid, err1 := strconv.ParseUint(vidStr, 16, 16)
	pid, err2 := strconv.ParseUint(pidStr, 16, 16)
	if err1 != nil || err2 != nil {
		panic(fmt.Sprintf("kb: %s: key %q is not hex", where, s))
	}
	return productKey(uint16(vid), uint16(pid))
}
