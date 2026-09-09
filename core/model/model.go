// Package model is the platform-neutral domain model for Port Authority.
//
// Nothing in this package may import a platform package. Providers
// (platform/*) produce these types; the insight engine, API and UI
// consume them. Every JSON shape here is part of the public API surface,
// so field names are stable and snake_case.
package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// SchemaVersion is stamped on every Topology so API consumers can detect
// shape changes.
const SchemaVersion = 1

// Bitrate is a bandwidth in bits per second.
type Bitrate int64

const (
	Kbps Bitrate = 1_000
	Mbps Bitrate = 1_000 * Kbps
	Gbps Bitrate = 1_000 * Mbps
)

func (b Bitrate) String() string {
	switch {
	case b >= Gbps:
		return fmt.Sprintf("%.3g Gbps", float64(b)/float64(Gbps))
	case b >= Mbps:
		return fmt.Sprintf("%.3g Mbps", float64(b)/float64(Mbps))
	case b >= Kbps:
		return fmt.Sprintf("%.3g Kbps", float64(b)/float64(Kbps))
	}
	return fmt.Sprintf("%d bps", int64(b))
}

// LinkSpeed is a USB / USB4 signalling rate. The ordering is meaningful:
// a higher value is a faster link, so comparisons like
// negotiated < claimed are valid.
type LinkSpeed int

const (
	LinkUnknown      LinkSpeed = iota
	LinkNone                   // nothing connected
	LinkLow                    // USB 1.x low speed, 1.5 Mbps
	LinkFull                   // USB 1.x full speed, 12 Mbps
	LinkHigh                   // USB 2.0 high speed, 480 Mbps
	LinkSuper                  // USB 3.x Gen 1, 5 Gbps
	LinkSuperPlus              // USB 3.x Gen 2, 10 Gbps
	LinkSuperPlus2x2           // USB 3.2 Gen 2x2, 20 Gbps
	LinkUSB4Gen2               // USB4 / TBT3 20 Gbps
	LinkUSB4Gen3               // USB4 / TBT3 40 Gbps
	LinkUSB4Gen4               // USB4 v2 80 Gbps
)

var linkSpeedInfo = map[LinkSpeed]struct {
	name string
	rate Bitrate
}{
	LinkUnknown:      {"unknown", 0},
	LinkNone:         {"none", 0},
	LinkLow:          {"low", 1_500 * Kbps},
	LinkFull:         {"full", 12 * Mbps},
	LinkHigh:         {"high", 480 * Mbps},
	LinkSuper:        {"ss5", 5 * Gbps},
	LinkSuperPlus:    {"ss10", 10 * Gbps},
	LinkSuperPlus2x2: {"ss20", 20 * Gbps},
	LinkUSB4Gen2:     {"usb4_20", 20 * Gbps},
	LinkUSB4Gen3:     {"usb4_40", 40 * Gbps},
	LinkUSB4Gen4:     {"usb4_80", 80 * Gbps},
}

func (l LinkSpeed) String() string {
	if info, ok := linkSpeedInfo[l]; ok {
		return info.name
	}
	return fmt.Sprintf("linkspeed(%d)", int(l))
}

// Bitrate is the nominal signalling rate of the link.
func (l LinkSpeed) Bitrate() Bitrate {
	return linkSpeedInfo[l].rate
}

// MarshalJSON renders the speed by name so fixtures and API payloads are
// readable and independent of the enum order.
func (l LinkSpeed) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.String())
}

func (l *LinkSpeed) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	for speed, info := range linkSpeedInfo {
		if info.name == name {
			*l = speed
			return nil
		}
	}
	return fmt.Errorf("unknown link speed %q", name)
}

// ControllerKind classifies a host controller.
type ControllerKind string

const (
	ControllerUnknown ControllerKind = "unknown"
	ControllerEHCI    ControllerKind = "ehci"
	ControllerXHCI    ControllerKind = "xhci"
	ControllerUSB4    ControllerKind = "usb4-router"
	ControllerTBT3    ControllerKind = "thunderbolt3"
)

// HubKind distinguishes the logical hubs Windows exposes. A physical
// USB 3 hub shows up as two logical hubs (a usb2 and a usb3 one) whose
// ports are linked by Connector.CompanionHubPath.
type HubKind string

const (
	HubUnknown HubKind = "unknown"
	HubRoot    HubKind = "root"
	HubUSB2    HubKind = "usb2"
	HubUSB3    HubKind = "usb3"
)

// DeviceClass is a coarse, human-meaningful device category.
type DeviceClass string

const (
	ClassUnknown   DeviceClass = "unknown"
	ClassHub       DeviceClass = "hub"
	ClassStorage   DeviceClass = "storage"
	ClassVideo     DeviceClass = "video"
	ClassAudio     DeviceClass = "audio"
	ClassHID       DeviceClass = "hid"
	ClassNetwork   DeviceClass = "network"
	ClassPrinter   DeviceClass = "printer"
	ClassSerial    DeviceClass = "serial"
	ClassWireless  DeviceClass = "wireless"
	ClassBillboard DeviceClass = "billboard"
	ClassDisplay   DeviceClass = "display" // USB displays (DisplayLink etc.), set from the knowledge base
	ClassImaging   DeviceClass = "imaging"
	ClassSmartCard DeviceClass = "smartcard"
	ClassVendor    DeviceClass = "vendor"
	ClassComposite DeviceClass = "composite"
)

// ConnectionStatus is the state of a hub port.
type ConnectionStatus string

const (
	StatusUnknown            ConnectionStatus = "unknown"
	StatusNoDevice           ConnectionStatus = "none"
	StatusConnected          ConnectionStatus = "connected"
	StatusFailedEnumeration  ConnectionStatus = "failed_enumeration"
	StatusGeneralFailure     ConnectionStatus = "general_failure"
	StatusOvercurrent        ConnectionStatus = "overcurrent"
	StatusNotEnoughPower     ConnectionStatus = "not_enough_power"
	StatusNotEnoughBandwidth ConnectionStatus = "not_enough_bandwidth"
	StatusNestedTooDeeply    ConnectionStatus = "nested_too_deeply"
	StatusLegacyHub          ConnectionStatus = "legacy_hub"
	StatusEnumerating        ConnectionStatus = "enumerating"
)

// Topology is a full snapshot of every USB controller on the machine.
type Topology struct {
	SchemaVersion int          `json:"schema_version"`
	CapturedAt    time.Time    `json:"captured_at"`
	Platform      string       `json:"platform"`
	Controllers   []Controller `json:"controllers"`
	// USB4 lists the USB4 / Thunderbolt routers (host and device) that
	// the OS exposes as a separate bus. See USB4Router for why they are
	// not folded into Controllers.
	USB4 []USB4Router `json:"usb4,omitempty"`
	// Warnings are non-fatal collection problems (a hub that would not
	// open, a descriptor that stalled). They are surfaced, never hidden.
	Warnings []string `json:"warnings,omitempty"`
}

// USB4Router is one router on the USB4 / Thunderbolt fabric: the host
// router in the computer, or a device router inside a dock or a USB4
// peripheral.
//
// Routers are kept separate from Controllers on purpose. A Controller is
// a USB host controller with a root hub and a port tree that the hub
// IOCTL walk can enumerate. A USB4 router is not a USB host: it tunnels
// PCIe, DisplayPort and USB 3 links, and the devices behind a PCIe
// tunnel (an NVMe SSD, a 10 GbE adapter) never appear in any hub tree.
// They surface on other buses (SCSI, PCI) and are listed here in
// Children so the topology can still say "this SSD is on a 40 Gbps
// port" after the device has vanished from the USB view.
type USB4Router struct {
	// ID is the PnP instance ID on Windows.
	ID         string `json:"id"`
	InstanceID string `json:"instance_id"`
	Name       string `json:"name"`
	// VendorID / ProductID are the USB4 router identifiers from the
	// router's configuration space. They are a different namespace from
	// USB VID/PID: the same product has a USB identity when it falls back
	// to USB 3 and a USB4 identity when it tunnels PCIe.
	VendorID  uint16 `json:"vendor_id"`
	ProductID uint16 `json:"product_id"`
	// VendorName / ProductName are resolved from the knowledge base, not
	// from the router. A router reports the silicon it is built on, so a
	// USB4 SSD announces its bridge chip rather than the product on the
	// desk; the knowledge base maps the router id back to the product.
	VendorName  string `json:"vendor_name,omitempty"`
	ProductName string `json:"product_name,omitempty"`
	// Kind is "host" for the router in the computer, "device" otherwise.
	Kind string `json:"kind"`
	// ParentID is the PnP parent: another router for device routers, the
	// PCI USB4 controller for the host router.
	ParentID string `json:"parent_id,omitempty"`
	// Depth is the number of USB4 routers between this one and the host
	// router (0 for the host router).
	Depth int `json:"depth"`
	// Children are the non-USB4 devices this router carries, rendered as
	// "friendly name (instance id)", e.g. the NVMe disk behind a PCIe
	// tunnel. USB devices behind a USB 3 tunnel are not listed here; they
	// are in the hub tree.
	Children []string `json:"children,omitempty"`
}

// Controller is an xHCI / USB4 root. Its root hub is a Device whose Hub
// field is populated.
type Controller struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Kind         ControllerKind `json:"kind"`
	MaxBandwidth Bitrate        `json:"max_bandwidth"`
	DevicePath   string         `json:"device_path,omitempty"`
	InstanceID   string         `json:"instance_id,omitempty"`
	DriverKey    string         `json:"driver_key,omitempty"`
	RootHub      *Device        `json:"root_hub"`
}

// Device is anything plugged into a port, including hubs. When the device
// is a hub, Hub is non-nil and carries its downstream ports.
type Device struct {
	// ID is stable across refreshes where the platform allows it (PnP
	// instance ID on Windows). It falls back to the port path.
	ID string `json:"id"`
	// PortPath is the physical route from the controller, e.g. "1/3/2"
	// meaning controller 1, root port 3, downstream port 2.
	PortPath string `json:"port_path"`

	VendorID  uint16 `json:"vendor_id"`
	ProductID uint16 `json:"product_id"`
	BCDDevice uint16 `json:"bcd_device"`
	BCDUSB    uint16 `json:"bcd_usb"`

	// Resolved from the knowledge base, not from the device.
	VendorName  string `json:"vendor_name,omitempty"`
	ProductName string `json:"product_name,omitempty"`

	Class       DeviceClass `json:"class"`
	USBClass    uint8       `json:"usb_class"`
	USBSubClass uint8       `json:"usb_subclass"`
	USBProtocol uint8       `json:"usb_protocol"`
	Interfaces  []Interface `json:"interfaces,omitempty"`

	// From string descriptors.
	Manufacturer string `json:"manufacturer,omitempty"`
	Product      string `json:"product,omitempty"`
	SerialNumber string `json:"serial_number,omitempty"`

	// From the OS device tree.
	Description  string `json:"description,omitempty"`
	FriendlyName string `json:"friendly_name,omitempty"`
	InstanceID   string `json:"instance_id,omitempty"`
	DriverKey    string `json:"driver_key,omitempty"`
	Location     string `json:"location,omitempty"`

	// ClaimedSpeed is what the device says it can do; the negotiated link
	// lives on the Port it is plugged into.
	ClaimedSpeed LinkSpeed `json:"claimed_speed"`
	// IsoReserved is the estimated isochronous bandwidth reserved by open
	// pipes (webcams, audio). Estimated from endpoint descriptors.
	IsoReserved Bitrate `json:"iso_reserved"`
	PowerDrawMA int     `json:"power_draw_ma"`
	// Children are OS-level things this device exposes: drive letters,
	// COM ports, camera names. Populated in Phase 1.
	Children    []string     `json:"children,omitempty"`
	TruthReport *TruthReport `json:"truth_report,omitempty"`
	Hub         *Hub         `json:"hub,omitempty"`
	// Enclosure is the physical box this hub lives in, filled by
	// enrichment. Only hubs carry one; a leaf device belongs to whatever
	// box its port is on.
	Enclosure *Enclosure `json:"enclosure,omitempty"`
}

// EnclosureKind is what sort of box an Enclosure describes.
type EnclosureKind string

const (
	// EnclosureHost is the computer itself: every controller and root hub.
	EnclosureHost EnclosureKind = "host"
	// EnclosureDock is a dock recognised in the knowledge base.
	EnclosureDock EnclosureKind = "dock"
	// EnclosureHub is any other box, i.e. a standalone hub.
	EnclosureHub EnclosureKind = "hub"
)

// Enclosure is the physical box a set of logical hubs lives in.
//
// A dock reports its internals as a chain of four or five hubs, and a
// laptop reports its own as several controllers and root hubs, but a
// person sees one object with sockets on it. Hubs that share an Enclosure
// ID are that one object, which is what lets the diagram draw the box
// instead of the chain.
type Enclosure struct {
	// ID is unique per box within a snapshot, so two docks of the same
	// model never merge into one.
	ID   string        `json:"id"`
	Kind EnclosureKind `json:"kind"`
	// Name is the box's name when the knowledge base knows it, e.g.
	// "CalDigit TS4". Empty for the host and for unrecognised hubs, which
	// the UI names from the machine or the hub device itself.
	Name string `json:"name,omitempty"`
	// DockID is the knowledge-base dock entry this box matched.
	DockID string `json:"dock_id,omitempty"`
}

// Interface is one USB interface descriptor, used to classify composite
// devices.
type Interface struct {
	Number    int         `json:"number"`
	Class     uint8       `json:"class"`
	SubClass  uint8       `json:"subclass"`
	Protocol  uint8       `json:"protocol"`
	Kind      DeviceClass `json:"kind"`
	Endpoints int         `json:"endpoints"`
}

// Hub is the downstream side of a hub device.
type Hub struct {
	Kind       HubKind `json:"kind"`
	Depth      int     `json:"depth"` // 0 for a root hub
	PortCount  int     `json:"port_count"`
	BusPowered bool    `json:"bus_powered"`
	DevicePath string  `json:"device_path,omitempty"`
	Ports      []Port  `json:"ports"`
}

// Port is one downstream port of a hub.
type Port struct {
	Number         int              `json:"number"`
	Status         ConnectionStatus `json:"status"`
	NegotiatedLink LinkSpeed        `json:"negotiated_link"`
	MaxLink        LinkSpeed        `json:"max_link"`
	Connector      *Connector       `json:"connector,omitempty"`
	// Label and Position are the printed label and placement (front/rear)
	// of the physical socket from the dock knowledge base, when known.
	Label    string       `json:"label,omitempty"`
	Position string       `json:"position,omitempty"`
	AltMode  *AltModeInfo `json:"alt_mode,omitempty"`
	Device   *Device      `json:"device,omitempty"`
}

// Connector describes the physical socket behind a logical port.
type Connector struct {
	TypeC              bool `json:"type_c"`
	UserConnectable    bool `json:"user_connectable"`
	MultipleCompanions bool `json:"multiple_companions"`
	// CompanionHubPath and CompanionPort identify the other logical hub
	// port (usb2 <-> usb3) sharing this physical connector.
	CompanionHubPath string `json:"companion_hub_path,omitempty"`
	CompanionPort    int    `json:"companion_port,omitempty"`
}

// AltModeInfo is DisplayPort alt-mode state on a Type-C port.
type AltModeInfo struct {
	DisplayPortLanes int     `json:"displayport_lanes"`
	DSC              bool    `json:"dsc"`
	Confidence       float64 `json:"confidence"`
	Source           string  `json:"source"` // "reported" or "inferred"
}

// TruthReport is the output of the truth tester.
type TruthReport struct {
	ClaimedSpeed     LinkSpeed `json:"claimed_speed"`
	MeasuredReadBps  Bitrate   `json:"measured_read_bps"`
	MeasuredWriteBps Bitrate   `json:"measured_write_bps"`
	CapacityVerdict  string    `json:"capacity_verdict"`
	Confidence       float64   `json:"confidence"`
	TestedAt         time.Time `json:"tested_at"`
}

// ProviderCaps declares what a platform provider can actually measure.
// Consumers degrade on these flags, never on the OS name.
type ProviderCaps struct {
	Platform          string `json:"platform"`
	Topology          bool   `json:"topology"`
	Hotplug           bool   `json:"hotplug"`
	Throughput        bool   `json:"throughput"`
	AltMode           bool   `json:"alt_mode"`
	PowerDraw         bool   `json:"power_draw"`
	IsoReservation    bool   `json:"iso_reservation"`
	ConnectorInfo     bool   `json:"connector_info"`
	StringDescriptors bool   `json:"string_descriptors"`
}

// EventKind is the type of a TopologyEvent.
type EventKind string

const (
	EventDeviceAdded   EventKind = "device_added"
	EventDeviceRemoved EventKind = "device_removed"
	EventLinkChanged   EventKind = "link_changed"
	EventResnapshot    EventKind = "resnapshot"
)

// TopologyEvent is a hotplug or link-change notification.
type TopologyEvent struct {
	Kind     EventKind `json:"kind"`
	At       time.Time `json:"at"`
	DeviceID string    `json:"device_id,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}

// ThroughputSample is a live throughput reading for one device.
type ThroughputSample struct {
	DeviceID string    `json:"device_id"`
	At       time.Time `json:"at"`
	ReadBps  Bitrate   `json:"read_bps"`
	WriteBps Bitrate   `json:"write_bps"`
}

// Walk visits every device in the topology depth-first. parent is nil for
// root hubs; port is nil for root hubs.
func (t *Topology) Walk(fn func(c *Controller, parent *Device, port *Port, d *Device)) {
	for i := range t.Controllers {
		c := &t.Controllers[i]
		if c.RootHub == nil {
			continue
		}
		fn(c, nil, nil, c.RootHub)
		walkHub(c, c.RootHub, fn)
	}
}

func walkHub(c *Controller, hubDev *Device, fn func(*Controller, *Device, *Port, *Device)) {
	if hubDev.Hub == nil {
		return
	}
	for i := range hubDev.Hub.Ports {
		p := &hubDev.Hub.Ports[i]
		if p.Device == nil {
			continue
		}
		fn(c, hubDev, p, p.Device)
		walkHub(c, p.Device, fn)
	}
}
