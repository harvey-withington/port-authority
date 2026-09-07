//go:build windows

// Package win is the Windows Provider: SetupAPI + hub IOCTLs for
// topology. Hotplug (CM_Register_Notification) and throughput (ETW) are
// later Phase 0/1 work and currently report ErrUnsupported.
package win

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"portauthority/core/model"
)

// Provider collects USB topology on Windows.
type Provider struct {
	restartMu         sync.Mutex
	restartThroughput chan<- struct{}
}

// New returns the Windows provider.
func New() *Provider { return &Provider{} }

// Capabilities declares what this provider can measure. Watch and
// Throughput live in hotplug.go and throughput.go.
func (p *Provider) Capabilities() model.ProviderCaps {
	return model.ProviderCaps{
		Platform:          "windows",
		Topology:          true,
		Hotplug:           true,
		Throughput:        p.throughputAvailable(),
		PowerDraw:         true,
		IsoReservation:    true,
		ConnectorInfo:     true,
		StringDescriptors: true,
	}
}

// Snapshot walks every host controller's root hub recursively.
func (p *Provider) Snapshot(ctx context.Context) (*model.Topology, error) {
	w := &walker{}
	idx, err := loadPnPIndex()
	if err != nil {
		w.warnf("pnp index unavailable, names will be missing: %v", err)
	}
	w.pnp = idx

	paths, err := deviceInterfaces(&guidUSBHostController)
	if err != nil {
		return nil, fmt.Errorf("enumerate host controllers: %w", err)
	}
	t := &model.Topology{
		SchemaVersion: model.SchemaVersion,
		CapturedAt:    time.Now(),
		Platform:      "windows",
	}
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, err := w.collectController(ctx, i+1, path)
		if err != nil {
			w.warnf("controller %s: %v", path, err)
			continue
		}
		t.Controllers = append(t.Controllers, c)
	}
	w.collectUSB4(t)
	t.Warnings = w.warnings
	return t, nil
}

type walker struct {
	pnp      *pnpIndex
	warnings []string
}

func (w *walker) warnf(format string, args ...any) {
	w.warnings = append(w.warnings, fmt.Sprintf(format, args...))
}

func (w *walker) collectController(ctx context.Context, ordinal int, path string) (model.Controller, error) {
	h, err := openDevice(path)
	if err != nil {
		return model.Controller{}, err
	}
	defer windows.CloseHandle(h)

	c := model.Controller{
		ID:         fmt.Sprintf("controller-%d", ordinal),
		Name:       path,
		Kind:       model.ControllerUnknown,
		DevicePath: path,
		InstanceID: instanceIDFromInterface(path),
	}
	if key, err := readNameNoIndex(h, ioctlGetHCDDriverKeyName); err == nil {
		c.DriverKey = key
	}
	info, ok := w.pnp.byDriver(c.DriverKey)
	if !ok {
		info, ok = w.pnp.byInstanceOrLookup(c.InstanceID)
	}
	if ok {
		if name := info.displayName(); name != "" {
			c.Name = name
		}
		if info.InstanceID != "" {
			c.InstanceID = info.InstanceID
			c.ID = info.InstanceID
		}
	}
	c.Kind, c.MaxBandwidth = controllerKind(c.Name)

	rootName, err := readNameNoIndex(h, ioctlUSBGetRootHubName)
	if err != nil {
		return c, fmt.Errorf("root hub name: %w", err)
	}
	root := &model.Device{
		ID:       "path:" + strconv.Itoa(ordinal),
		PortPath: strconv.Itoa(ordinal),
		Class:    model.ClassHub,
		USBClass: 9,
		Hub:      &model.Hub{Kind: model.HubRoot, Depth: 0},
	}
	if info, ok := w.pnp.byInstance(instanceIDFromInterface(rootName)); ok {
		applyPnP(root, info)
	}
	w.walkHub(ctx, root, rootName)
	c.RootHub = root
	return c, nil
}

// walkHub opens the hub at symbolic link name and fills dev.Hub.Ports.
func (w *walker) walkHub(ctx context.Context, dev *model.Device, name string) {
	hub := dev.Hub
	hub.DevicePath = name
	h, err := openDevice(`\\.\` + name)
	if err != nil {
		w.warnf("hub %s: %v", dev.PortPath, err)
		return
	}
	defer windows.CloseHandle(h)

	// USB_NODE_INFORMATION (packed, 76 bytes):
	//   0 ULONG NodeType
	//   4 USB_HUB_DESCRIPTOR: 4 bLength, 5 bType, 6 bNumberOfPorts,
	//     7 wHubCharacteristics, 9 bPwrOn2PwrGood, 10 bHubContrCurrent,
	//     11 bRemoveAndPowerMask[64]
	//  75 BOOLEAN HubIsBusPowered
	node := make([]byte, 76)
	if _, err := ioctl(h, ioctlUSBGetNodeInformation, node, node); err != nil {
		w.warnf("hub %s: node information: %v", dev.PortPath, err)
		return
	}
	hub.PortCount = int(node[6])
	hub.BusPowered = node[75] != 0

	// USB_HUB_INFORMATION_EX (packed): 0 ULONG HubType, 4 USHORT HighestPortNumber
	hubEx := make([]byte, 77)
	if _, err := ioctl(h, ioctlUSBGetHubInformationEx, hubEx, hubEx); err == nil {
		switch binary.LittleEndian.Uint32(hubEx[0:]) {
		case 1:
			hub.Kind = model.HubRoot
		case 2:
			hub.Kind = model.HubUSB2
		case 3:
			hub.Kind = model.HubUSB3
		}
		if n := int(binary.LittleEndian.Uint16(hubEx[4:])); n > 0 {
			hub.PortCount = n
		}
	}
	if hub.Kind == "" {
		hub.Kind = model.HubUnknown
	}

	for port := 1; port <= hub.PortCount; port++ {
		if ctx.Err() != nil {
			return
		}
		hub.Ports = append(hub.Ports, w.collectPort(ctx, h, dev, port))
	}
}

// Offsets into USB_NODE_CONNECTION_INFORMATION_EX (packed, 35 bytes + pipes):
//
//	 0 ULONG ConnectionIndex
//	 4 USB_DEVICE_DESCRIPTOR (18 bytes)
//	22 UCHAR CurrentConfigurationValue
//	23 UCHAR Speed (0 low, 1 full, 2 high, 3 super)
//	24 BOOLEAN DeviceIsHub
//	25 USHORT DeviceAddress
//	27 ULONG NumberOfOpenPipes
//	31 ULONG ConnectionStatus
//	35 USB_PIPE_INFO PipeList[] (11 bytes each: 7-byte endpoint descriptor + ULONG ScheduleOffset)
const (
	connInfoHeader = 35
	pipeInfoSize   = 11
	maxPipes       = 32
)

func (w *walker) collectPort(ctx context.Context, h windows.Handle, parent *model.Device, number int) model.Port {
	portPath := parent.PortPath + "/" + strconv.Itoa(number)
	port := model.Port{Number: number, Status: model.StatusUnknown, MaxLink: model.LinkUnknown, NegotiatedLink: model.LinkNone}

	buf := make([]byte, connInfoHeader+maxPipes*pipeInfoSize)
	binary.LittleEndian.PutUint32(buf[0:], uint32(number))
	if _, err := ioctl(h, ioctlUSBGetNodeConnectionInformationEx, buf, buf); err != nil {
		w.warnf("port %s: connection info: %v", portPath, err)
		return port
	}
	status := binary.LittleEndian.Uint32(buf[31:])
	port.Status = connectionStatus(status)

	// V2: port-level protocol support and SuperSpeed(+) state.
	// USB_NODE_CONNECTION_INFORMATION_EX_V2 (16 bytes):
	//   0 ConnectionIndex, 4 Length, 8 SupportedUsbProtocols, 12 Flags
	var protocols, flags uint32
	v2 := make([]byte, 16)
	binary.LittleEndian.PutUint32(v2[0:], uint32(number))
	binary.LittleEndian.PutUint32(v2[4:], 16)
	binary.LittleEndian.PutUint32(v2[8:], 0x7) // Usb110|Usb200|Usb300 understood
	haveV2 := false
	if _, err := ioctl(h, ioctlUSBGetNodeConnectionInformationExV2, v2, v2); err == nil {
		haveV2 = true
		protocols = binary.LittleEndian.Uint32(v2[8:])
		flags = binary.LittleEndian.Uint32(v2[12:])
	}
	port.MaxLink = portMaxLink(haveV2, protocols, parent.Hub.Kind)
	port.Connector = w.connectorProperties(h, number)

	if status != 1 { // DeviceConnected
		return port
	}

	dev := &model.Device{ID: "path:" + portPath, PortPath: portPath}
	desc := buf[4:22]
	dev.BCDUSB = binary.LittleEndian.Uint16(desc[2:])
	dev.USBClass, dev.USBSubClass, dev.USBProtocol = desc[4], desc[5], desc[6]
	dev.VendorID = binary.LittleEndian.Uint16(desc[8:])
	dev.ProductID = binary.LittleEndian.Uint16(desc[10:])
	dev.BCDDevice = binary.LittleEndian.Uint16(desc[12:])
	iManufacturer, iProduct, iSerial := desc[14], desc[15], desc[16]
	speed := buf[23]
	isHub := buf[24] != 0
	openPipes := int(binary.LittleEndian.Uint32(buf[27:]))

	port.NegotiatedLink = negotiatedLink(speed, haveV2, flags)
	if port.MaxLink < port.NegotiatedLink {
		// Windows only reports the protocol generation per port, so a
		// Gen2 port looks like ss5 until something negotiates ss10 on it.
		port.MaxLink = port.NegotiatedLink
	}
	dev.ClaimedSpeed = claimedLink(port.NegotiatedLink, haveV2, flags, dev.BCDUSB)
	dev.Class = classFromUSB(dev.USBClass, dev.USBSubClass, dev.USBProtocol)
	if isHub {
		dev.Class = model.ClassHub
	}

	if openPipes > maxPipes {
		openPipes = maxPipes
	}
	for i := 0; i < openPipes; i++ {
		ep := buf[connInfoHeader+i*pipeInfoSize:]
		dev.IsoReserved += isoBandwidth(ep[:7], port.NegotiatedLink)
	}

	if key, err := readNameWithIndex(h, ioctlUSBGetNodeConnectionDriverKeyName, uint32(number)); err == nil {
		dev.DriverKey = key
		if info, ok := w.pnp.byDriver(key); ok {
			applyPnP(dev, info)
		}
	}

	lang := firstLanguage(h, uint32(number))
	dev.Manufacturer = w.readString(h, number, iManufacturer, lang)
	dev.Product = w.readString(h, number, iProduct, lang)
	dev.SerialNumber = w.readString(h, number, iSerial, lang)

	if cfg, err := configDescriptor(h, uint32(number)); err == nil {
		applyConfigDescriptor(dev, cfg)
	} else {
		w.warnf("port %s: config descriptor: %v", portPath, err)
	}

	if isHub {
		dev.Hub = &model.Hub{Kind: model.HubUnknown, Depth: parent.Hub.Depth + 1}
		name, err := readNameWithIndex(h, ioctlUSBGetNodeConnectionName, uint32(number))
		if err != nil {
			w.warnf("port %s: hub name: %v", portPath, err)
		} else {
			w.walkHub(ctx, dev, name)
		}
	}
	port.Device = dev
	return port
}

func (w *walker) readString(h windows.Handle, number int, strIndex uint8, lang uint16) string {
	if strIndex == 0 {
		return ""
	}
	s, err := stringDescriptor(h, uint32(number), strIndex, lang)
	if err != nil {
		// Stalled string descriptors are common on cheap devices; not worth a warning.
		return ""
	}
	return s
}

// connectorProperties reads USB_PORT_CONNECTOR_PROPERTIES (packed):
//
//	 0 ULONG ConnectionIndex, 4 ULONG ActualLength, 8 ULONG UsbPortProperties,
//	12 USHORT CompanionIndex, 14 USHORT CompanionPortNumber, 16 WCHAR CompanionHubSymbolicLinkName[]
func (w *walker) connectorProperties(h windows.Handle, number int) *model.Connector {
	probe := make([]byte, 16)
	binary.LittleEndian.PutUint32(probe[0:], uint32(number))
	if _, err := ioctl(h, ioctlUSBGetPortConnectorProperties, probe, probe); err != nil {
		return nil
	}
	total := binary.LittleEndian.Uint32(probe[4:])
	buf := probe
	if total > 16 && total < 1<<16 {
		buf = make([]byte, total)
		binary.LittleEndian.PutUint32(buf[0:], uint32(number))
		if _, err := ioctl(h, ioctlUSBGetPortConnectorProperties, buf, buf); err != nil {
			buf = probe
		}
	}
	props := binary.LittleEndian.Uint32(buf[8:])
	c := &model.Connector{
		UserConnectable:    props&1 != 0,
		MultipleCompanions: props&4 != 0,
		TypeC:              props&8 != 0,
		CompanionPort:      int(binary.LittleEndian.Uint16(buf[14:])),
	}
	if len(buf) > 16 {
		c.CompanionHubPath = decodeWide(buf[16:])
	}
	return c
}

func applyPnP(dev *model.Device, info pnpInfo) {
	dev.InstanceID = info.InstanceID
	dev.Description = info.Description
	dev.FriendlyName = info.FriendlyName
	dev.Location = info.Location
	if info.DriverKey != "" {
		dev.DriverKey = info.DriverKey
	}
	if info.InstanceID != "" {
		dev.ID = info.InstanceID
	}
}

// applyConfigDescriptor extracts bMaxPower and the interface list.
//
// Configuration descriptor: 0 bLength, 1 bType(2), 2 wTotalLength,
// 4 bNumInterfaces, 5 bConfigurationValue, 6 iConfiguration,
// 7 bmAttributes, 8 bMaxPower (2 mA units; 8 mA units at SuperSpeed).
// Interface descriptor (type 4): 2 bInterfaceNumber, 3 bAlternateSetting,
// 4 bNumEndpoints, 5 bInterfaceClass, 6 bInterfaceSubClass, 7 bInterfaceProtocol.
func applyConfigDescriptor(dev *model.Device, cfg []byte) {
	if len(cfg) < 9 {
		return
	}
	unit := 2
	if dev.ClaimedSpeed >= model.LinkSuper {
		unit = 8
	}
	dev.PowerDrawMA = int(cfg[8]) * unit

	for off := int(cfg[0]); off+2 <= len(cfg); {
		length, typ := int(cfg[off]), cfg[off+1]
		if length < 2 || off+length > len(cfg) {
			break
		}
		if typ == 4 && length >= 9 && cfg[off+3] == 0 { // alternate setting 0 only
			dev.Interfaces = append(dev.Interfaces, model.Interface{
				Number:    int(cfg[off+2]),
				Endpoints: int(cfg[off+4]),
				Class:     cfg[off+5],
				SubClass:  cfg[off+6],
				Protocol:  cfg[off+7],
				Kind:      classFromUSB(cfg[off+5], cfg[off+6], cfg[off+7]),
			})
		}
		off += length
	}
	if dev.Class == model.ClassUnknown || dev.Class == model.ClassComposite {
		dev.Class = classFromInterfaces(dev.Interfaces)
	}
}

// isoBandwidth estimates the bandwidth an open isochronous endpoint
// reserves, from its 7-byte endpoint descriptor:
// 2 bEndpointAddress, 3 bmAttributes, 4 wMaxPacketSize, 6 bInterval.
func isoBandwidth(ep []byte, link model.LinkSpeed) model.Bitrate {
	if len(ep) < 7 || ep[3]&3 != 1 {
		return 0
	}
	mps := binary.LittleEndian.Uint16(ep[4:])
	size := int64(mps & 0x7ff)
	mult := int64((mps>>11)&3) + 1
	interval := int64(ep[6])
	switch link {
	case model.LinkLow, model.LinkFull:
		if interval < 1 {
			interval = 1
		}
		return model.Bitrate(size * 8 * 1000 / interval) // packets per 1 ms frame
	default:
		if interval < 1 {
			interval = 1
		}
		if interval > 16 {
			interval = 16
		}
		frames := int64(1) << (interval - 1) // 125 us microframes
		return model.Bitrate(size * mult * 8 * 8000 / frames)
	}
}

func connectionStatus(s uint32) model.ConnectionStatus {
	switch s {
	case 0:
		return model.StatusNoDevice
	case 1:
		return model.StatusConnected
	case 2:
		return model.StatusFailedEnumeration
	case 3:
		return model.StatusGeneralFailure
	case 4:
		return model.StatusOvercurrent
	case 5:
		return model.StatusNotEnoughPower
	case 6:
		return model.StatusNotEnoughBandwidth
	case 7:
		return model.StatusNestedTooDeeply
	case 8:
		return model.StatusLegacyHub
	case 9:
		return model.StatusEnumerating
	}
	return model.StatusUnknown
}

// negotiatedLink combines the legacy Speed byte with the V2 flags.
// Flags: bit0 operating at SS+, bit1 SS capable, bit2 operating at SS+ (Gen2), bit3 SS+ capable.
func negotiatedLink(speed uint8, haveV2 bool, flags uint32) model.LinkSpeed {
	if haveV2 {
		if flags&4 != 0 {
			return model.LinkSuperPlus
		}
		if flags&1 != 0 {
			return model.LinkSuper
		}
	}
	switch speed {
	case 0:
		return model.LinkLow
	case 1:
		return model.LinkFull
	case 2:
		return model.LinkHigh
	case 3:
		return model.LinkSuper
	}
	return model.LinkUnknown
}

// claimedLink is the best link the device says it supports. Without V2
// data we cannot tell a full-speed-only USB 2.0 device from a high-speed
// one, so the negotiated link is the floor.
func claimedLink(negotiated model.LinkSpeed, haveV2 bool, flags uint32, bcdUSB uint16) model.LinkSpeed {
	claimed := negotiated
	if haveV2 {
		if flags&8 != 0 && claimed < model.LinkSuperPlus {
			claimed = model.LinkSuperPlus
		} else if flags&2 != 0 && claimed < model.LinkSuper {
			claimed = model.LinkSuper
		}
	}
	if bcdUSB >= 0x0300 && claimed < model.LinkSuper {
		claimed = model.LinkSuper
	}
	return claimed
}

// portMaxLink is what the port itself can signal. Windows only reports
// the protocol generation, so a Gen2 port shows as ss5 here; the
// knowledge base refines this from the controller model.
func portMaxLink(haveV2 bool, protocols uint32, hubKind model.HubKind) model.LinkSpeed {
	if haveV2 {
		switch {
		case protocols&4 != 0:
			return model.LinkSuper
		case protocols&2 != 0:
			return model.LinkHigh
		case protocols&1 != 0:
			return model.LinkFull
		}
	}
	if hubKind == model.HubUSB3 {
		return model.LinkSuper
	}
	return model.LinkHigh
}

func classFromUSB(class, sub, proto uint8) model.DeviceClass {
	switch class {
	case 0x00:
		return model.ClassComposite
	case 0x01:
		return model.ClassAudio
	case 0x02:
		if sub == 0x06 || sub == 0x0D {
			return model.ClassNetwork // ECM / NCM
		}
		return model.ClassSerial
	case 0x03:
		return model.ClassHID
	case 0x06:
		return model.ClassImaging
	case 0x07:
		return model.ClassPrinter
	case 0x08:
		return model.ClassStorage
	case 0x09:
		return model.ClassHub
	case 0x0A:
		return model.ClassSerial // CDC data
	case 0x0B:
		return model.ClassSmartCard
	case 0x0E:
		return model.ClassVideo
	case 0x10:
		return model.ClassAudio // audio/video
	case 0x11:
		return model.ClassBillboard
	case 0xE0:
		return model.ClassWireless
	case 0xEF:
		return model.ClassComposite
	case 0xFF:
		return model.ClassVendor
	}
	return model.ClassUnknown
}

// classFromInterfaces picks the most bandwidth-relevant class of a
// composite device: storage and video dominate, then audio, then the rest.
func classFromInterfaces(ifaces []model.Interface) model.DeviceClass {
	priority := []model.DeviceClass{
		model.ClassStorage, model.ClassVideo, model.ClassNetwork, model.ClassAudio,
		model.ClassImaging, model.ClassPrinter, model.ClassSerial, model.ClassHID,
		model.ClassWireless, model.ClassSmartCard, model.ClassVendor,
	}
	for _, want := range priority {
		for _, i := range ifaces {
			if i.Kind == want {
				return want
			}
		}
	}
	if len(ifaces) > 0 {
		return model.ClassComposite
	}
	return model.ClassUnknown
}

// controllerKind is a name heuristic. The knowledge base will replace this
// with VID/PID keyed data; it only has to be roughly right for the spike.
func controllerKind(name string) (model.ControllerKind, model.Bitrate) {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "usb4"):
		return model.ControllerUSB4, 40 * model.Gbps
	case strings.Contains(n, "thunderbolt"):
		return model.ControllerTBT3, 40 * model.Gbps
	case strings.Contains(n, "xhci") || strings.Contains(n, "extensible"):
		switch {
		case strings.Contains(n, "3.2") || strings.Contains(n, "3.1") || strings.Contains(n, "3.20") || strings.Contains(n, "3.10"):
			return model.ControllerXHCI, 10 * model.Gbps
		default:
			return model.ControllerXHCI, 5 * model.Gbps
		}
	case strings.Contains(n, "ehci") || strings.Contains(n, "enhanced"):
		return model.ControllerEHCI, 480 * model.Mbps
	}
	return model.ControllerUnknown, 0
}
