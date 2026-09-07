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

	"portauthority/core/model"
)

var (
	procCMGetParent    = modcfgmgr32.NewProc("CM_Get_Parent")
	procCMGetChild     = modcfgmgr32.NewProc("CM_Get_Child")
	procCMGetSibling   = modcfgmgr32.NewProc("CM_Get_Sibling")
	procCMGetDeviceIDW = modcfgmgr32.NewProc("CM_Get_Device_IDW")
)

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
	vendor, product, ok := usb4NameParts(r.Name)
	if !ok {
		return nil
	}
	var out []string
	for _, info := range w.pnp.byInstanceID {
		upper := strings.ToUpper(info.InstanceID)
		if !strings.HasPrefix(upper, `SCSI\`) && !strings.HasPrefix(upper, `NVME\`) && !strings.HasPrefix(upper, `PCI\`) {
			continue
		}
		name := normalizeName(info.displayName())
		if strings.Contains(name, vendor) && strings.Contains(name, product) {
			out = append(out, w.pnpLabel(info.InstanceID))
		}
	}
	sort.Strings(out)
	return out
}

// usb4NameParts splits "USB4 Router (2.0), Corsair - EX400U" into a
// normalized vendor token ("corsair") and product token ("ex400u").
func usb4NameParts(name string) (vendor, product string, ok bool) {
	_, rest, found := strings.Cut(name, ",")
	if !found {
		return "", "", false
	}
	v, p, found := strings.Cut(rest, " - ")
	if !found {
		return "", "", false
	}
	vendorWords := strings.Fields(normalizeName(v))
	product = strings.TrimSpace(normalizeName(p))
	if len(vendorWords) == 0 || len(product) < 3 {
		return "", "", false
	}
	return vendorWords[0], product, true
}

// normalizeName lower-cases and keeps only letters, digits and spaces so
// "CalDigit. Inc." and "CALDIGIT_INC" compare equal.
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == ' ' || r == '.':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
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
