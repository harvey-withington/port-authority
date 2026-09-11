//go:build windows

package win

// USB4 / Thunderbolt routers. Windows enumerates them under the "USB4"
// enumerator (usb4devicerouter.inf), one node per router: the host
// router in the computer, then one device router per dock or USB4
// peripheral. They are not USB hosts and have no hub IOCTL surface, so
// they are collected from PnP alone.
//
// A device that tunnels PCIe (an NVMe SSD, a network adapter) does not
// hang below its router in the PnP tree. On this machine the EX400U's
// NVMe controller sits under a PCI Express switch port of the dock's
// Goshen Ridge, with the disk below it; the router node has no PnP
// children at all. The direct CM_Get_Child walk is kept for platforms
// and drivers that do link them, and a name match against the SCSI /
// NVME / PCI buses fills in the tunneled endpoints otherwise.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"portauthority/core/kb"
	"portauthority/core/model"
)

var (
	procCMGetParent           = modcfgmgr32.NewProc("CM_Get_Parent")
	procCMGetChild            = modcfgmgr32.NewProc("CM_Get_Child")
	procCMGetSibling          = modcfgmgr32.NewProc("CM_Get_Sibling")
	procCMGetDeviceIDW        = modcfgmgr32.NewProc("CM_Get_Device_IDW")
	procCMGetDevNodePropertyW = modcfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
)

// usb4RouterProps is the property set the inbox USB4 driver
// (usb4devicerouter.inf) stamps on every router devnode. It is not
// documented; the keys were found by listing every property on a router
// (tools/usb4props) and reading the values against the USB4 spec's router
// and lane adapter configuration spaces, whose field encodings Linux names
// in drivers/thunderbolt/tb_regs.h. What each pid holds, as observed:
//
//	 9  DROM vendor string      "CalDigit, Inc."
//	10  DROM model string       "TS4"
//	13  USB vendor id (u16)     the product's USB identity
//	14  USB product id (u16)
//	16  USB4 vendor id (u16)    also in the instance id
//	17  USB4 product id (u16)
//	18  revision (u16)          also in the hardware id as REV_
//	20  current link speed      lane adapter CS_1: 0x8 Gen 2, 0x4 Gen 3, 0x2 Gen 4
//	21  current link width      lane adapter CS_1: 1 single, 2 dual
//
// A host router has no upstream link, so 20 and 21 are absent on it.
var usb4RouterProps = windows.GUID{
	Data1: 0x5DF7E321, Data2: 0x1C1B, Data3: 0x4CE2,
	Data4: [8]byte{0xB4, 0xFA, 0x55, 0xF4, 0xA5, 0xBC, 0x2C, 0xB6},
}

const (
	usb4PropVendor    uint32 = 9
	usb4PropModel     uint32 = 10
	usb4PropUSBVID    uint32 = 13
	usb4PropUSBPID    uint32 = 14
	usb4PropRevision  uint32 = 18
	usb4PropLinkSpeed uint32 = 20
	usb4PropLinkWidth uint32 = 21

	// DEVPROP_TYPE_* values from devpropdef.h.
	devPropTypeUint16 uint32 = 0x05
	devPropTypeUint32 uint32 = 0x07
	devPropTypeString uint32 = 0x12

	// Lane adapter current link speed encodings.
	laneSpeedGen2 uint32 = 0x8
	laneSpeedGen3 uint32 = 0x4
	laneSpeedGen4 uint32 = 0x2
)

// devPropKey mirrors DEVPROPKEY.
type devPropKey struct {
	fmtid windows.GUID
	pid   uint32
}

// devNodeProperty reads one property of a devnode; ok is false when the
// devnode does not carry it.
func devNodeProperty(devInst windows.DEVINST, key devPropKey) (typ uint32, data []byte, ok bool) {
	var size uint32
	procCMGetDevNodePropertyW.Call(uintptr(devInst), uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)), 0)
	if size == 0 {
		return 0, nil, false
	}
	data = make([]byte, size)
	ret, _, _ := procCMGetDevNodePropertyW.Call(uintptr(devInst), uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 0)
	if ret != crSuccess {
		return 0, nil, false
	}
	return typ, data[:size], true
}

func usb4PropString(devInst windows.DEVINST, pid uint32) string {
	typ, data, ok := devNodeProperty(devInst, devPropKey{usb4RouterProps, pid})
	if !ok || typ != devPropTypeString || len(data) < 2 {
		return ""
	}
	u := make([]uint16, len(data)/2)
	for i := range u {
		u[i] = uint16(data[2*i]) | uint16(data[2*i+1])<<8
	}
	return strings.TrimSpace(windows.UTF16ToString(u))
}

func usb4PropUint16(devInst windows.DEVINST, pid uint32) uint16 {
	typ, data, ok := devNodeProperty(devInst, devPropKey{usb4RouterProps, pid})
	if !ok || typ != devPropTypeUint16 || len(data) < 2 {
		return 0
	}
	return uint16(data[0]) | uint16(data[1])<<8
}

func usb4PropUint32(devInst windows.DEVINST, pid uint32) (uint32, bool) {
	typ, data, ok := devNodeProperty(devInst, devPropKey{usb4RouterProps, pid})
	if !ok || typ != devPropTypeUint32 || len(data) < 4 {
		return 0, false
	}
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24, true
}

// readUSB4Props fills what the router says about itself and its upstream
// link. Every field is optional: an older driver that lacks the property
// set leaves the router as the instance id alone describes it.
func readUSB4Props(devInst windows.DEVINST, r *model.USB4Router) {
	r.Vendor = usb4PropString(devInst, usb4PropVendor)
	r.Model = usb4PropString(devInst, usb4PropModel)
	r.USBVendorID = usb4PropUint16(devInst, usb4PropUSBVID)
	r.USBProductID = usb4PropUint16(devInst, usb4PropUSBPID)
	r.Revision = usb4PropUint16(devInst, usb4PropRevision)
	speed, ok := usb4PropUint32(devInst, usb4PropLinkSpeed)
	if !ok {
		return
	}
	switch speed {
	case laneSpeedGen2:
		r.LinkGen = 2
	case laneSpeedGen3:
		r.LinkGen = 3
	case laneSpeedGen4:
		r.LinkGen = 4
	}
	if width, ok := usb4PropUint32(devInst, usb4PropLinkWidth); ok && (width == 1 || width == 2) {
		r.LinkLanes = int(width)
	}
	r.NegotiatedLink = model.USB4LinkSpeed(r.LinkGen, r.LinkLanes)
}

const (
	crSuccess        = 0
	crNoSuchDevnode  = 0x0D
	cmMaxDeviceIDLen = 200
)

// collectUSB4 fills t.USB4. Problems become warnings; it never fails.
func (w *walker) collectUSB4(t *model.Topology) {
	set, err := windows.SetupDiGetClassDevsEx(nil, "USB4", 0,
		windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		w.warnf("usb4: enumerate routers: %v", err)
		return
	}
	defer set.Close()

	byID := map[string]*model.USB4Router{}
	var routers []*model.USB4Router
	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if err != nil {
			break
		}
		id, err := set.DeviceInstanceID(data)
		if err != nil {
			w.warnf("usb4: device %d: instance id: %v", i, err)
			continue
		}
		vid, pid, hasVID := usb4VIDPID(id)
		upper := strings.ToUpper(id)
		isHost := strings.Contains(upper, "HOST_ROUTER") || strings.Contains(upper, "ROOT_DEVICE_ROUTER")
		if !hasVID && !isHost {
			// e.g. USB4\VIRTUAL_POWER_PDO: the power coordination PDO,
			// not a router.
			continue
		}
		r := &model.USB4Router{
			ID:         id,
			InstanceID: id,
			Name:       regString(set, data, windows.SPDRP_FRIENDLYNAME),
			VendorID:   vid,
			ProductID:  pid,
			Kind:       "device",
		}
		if r.Name == "" {
			r.Name = regString(set, data, windows.SPDRP_DEVICEDESC)
		}
		if isHost {
			r.Kind = "host"
		}
		readUSB4Props(data.DevInst, r)
		if parent, err := cmParent(data.DevInst); err != nil {
			w.warnf("usb4: %s: parent: %v", id, err)
		} else {
			r.ParentID = parent
		}
		r.Children = w.usb4PnPChildren(id, data.DevInst, 2)
		byID[strings.ToLower(id)] = r
		routers = append(routers, r)
	}

	for _, r := range routers {
		r.Depth = usb4Depth(byID, r)
		if r.Kind == "device" {
			r.Children = appendUnique(r.Children, w.usb4TunneledChildren(r)...)
		}
	}
	sort.SliceStable(routers, func(i, j int) bool {
		if routers[i].Depth != routers[j].Depth {
			return routers[i].Depth < routers[j].Depth
		}
		return routers[i].InstanceID < routers[j].InstanceID
	})
	for _, r := range routers {
		t.USB4 = append(t.USB4, *r)
	}
}

// usb4Depth counts USB4 ancestors. A cycle or a missing parent ends the
// chain, so a device router with no host router above it is depth 0.
func usb4Depth(byID map[string]*model.USB4Router, r *model.USB4Router) int {
	depth := 0
	seen := map[*model.USB4Router]bool{r: true}
	for cur := r; ; {
		parent, ok := byID[strings.ToLower(cur.ParentID)]
		if !ok || seen[parent] {
			return depth
		}
		seen[parent] = true
		depth++
		cur = parent
	}
}

// usb4PnPChildren lists the non-USB4 PnP descendants of a router down to
// the given number of levels, as "name (instance id)".
func (w *walker) usb4PnPChildren(routerID string, dev windows.DEVINST, levels int) []string {
	if levels == 0 {
		return nil
	}
	var out []string
	child, err := cmChild(dev)
	if err != nil {
		w.warnf("usb4: %s: children: %v", routerID, err)
		return nil
	}
	for child != 0 {
		id, err := cmDeviceID(child)
		if err != nil {
			w.warnf("usb4: %s: child id: %v", routerID, err)
			break
		}
		if !isUSB4InstanceID(id) {
			out = append(out, w.pnpLabel(id))
			out = append(out, w.usb4PnPChildren(routerID, child, levels-1)...)
		}
		next, err := cmSibling(child)
		if err != nil {
			w.warnf("usb4: %s: sibling: %v", routerID, err)
			break
		}
		child = next
	}
	return out
}

// usb4TunneledChildren finds PCIe-tunneled endpoints for a device router.
// The inbox driver names a router "USB4 Router (2.0), <vendor> - <product>"
// from the router's configuration space strings; the same strings end up
// in the friendly name of the disk or adapter it carries. Only the buses a
// PCIe tunnel produces (SCSI, NVME, PCI) are searched, so USB devices,
// which are already in the hub tree, are never duplicated.
func (w *walker) usb4TunneledChildren(r *model.USB4Router) []string {
	if w.pnp == nil {
		return nil
	}
	vendor, product, ok := kb.ParseUSB4Name(r.Name)
	if !ok {
		return nil
	}
	var out []string
	for _, info := range w.pnp.byInstanceID {
		upper := strings.ToUpper(info.InstanceID)
		if !strings.HasPrefix(upper, `SCSI\`) && !strings.HasPrefix(upper, `NVME\`) && !strings.HasPrefix(upper, `PCI\`) {
			continue
		}
		name := kb.NormalizeName(info.displayName())
		if strings.Contains(name, vendor) && strings.Contains(name, product) {
			out = append(out, w.pnpLabel(info.InstanceID))
		}
	}
	sort.Strings(out)
	return out
}

func (w *walker) pnpLabel(id string) string {
	if info, ok := w.pnp.byInstance(id); ok {
		if name := info.displayName(); name != "" {
			return fmt.Sprintf("%s (%s)", name, info.InstanceID)
		}
	}
	return id
}

func isUSB4InstanceID(id string) bool {
	return strings.HasPrefix(strings.ToUpper(id), `USB4\`)
}

// usb4VIDPID parses VID_xxxx&PID_xxxx out of a USB4 instance id.
func usb4VIDPID(id string) (vid, pid uint16, ok bool) {
	upper := strings.ToUpper(id)
	v := hexAfter(upper, "VID_")
	p := hexAfter(upper, "PID_")
	if v < 0 || p < 0 {
		return 0, 0, false
	}
	return uint16(v), uint16(p), true
}

func hexAfter(s, marker string) int64 {
	i := strings.Index(s, marker)
	if i < 0 {
		return -1
	}
	rest := s[i+len(marker):]
	end := 0
	for end < len(rest) && end < 4 && strings.ContainsRune("0123456789ABCDEF", rune(rest[end])) {
		end++
	}
	if end == 0 {
		return -1
	}
	n, err := strconv.ParseInt(rest[:end], 16, 32)
	if err != nil {
		return -1
	}
	return n
}

func appendUnique(dst []string, more ...string) []string {
	seen := map[string]bool{}
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range more {
		if !seen[s] {
			seen[s] = true
			dst = append(dst, s)
		}
	}
	return dst
}

// --- cfgmgr32 -----------------------------------------------------------

func cmRelative(proc *windows.LazyProc, dev windows.DEVINST) (windows.DEVINST, error) {
	var out windows.DEVINST
	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(&out)), uintptr(dev), 0)
	switch ret {
	case crSuccess:
		return out, nil
	case crNoSuchDevnode:
		return 0, nil
	}
	return 0, fmt.Errorf("%s: CONFIGRET 0x%x", proc.Name, ret)
}

func cmChild(dev windows.DEVINST) (windows.DEVINST, error) { return cmRelative(procCMGetChild, dev) }
func cmSibling(dev windows.DEVINST) (windows.DEVINST, error) {
	return cmRelative(procCMGetSibling, dev)
}

// cmParent returns the parent's instance id, or "" at the root.
func cmParent(dev windows.DEVINST) (string, error) {
	parent, err := cmRelative(procCMGetParent, dev)
	if err != nil || parent == 0 {
		return "", err
	}
	return cmDeviceID(parent)
}

func cmDeviceID(dev windows.DEVINST) (string, error) {
	buf := make([]uint16, cmMaxDeviceIDLen+1)
	ret, _, _ := procCMGetDeviceIDW.Call(uintptr(dev), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if ret != crSuccess {
		return "", fmt.Errorf("CM_Get_Device_IDW: CONFIGRET 0x%x", ret)
	}
	return windows.UTF16ToString(buf), nil
}
