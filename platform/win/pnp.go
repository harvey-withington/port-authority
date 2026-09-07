//go:build windows

package win

// Plug and Play device tree index. Hub IOCTLs identify devices by driver
// key name; SetupAPI is the only place that maps a driver key back to an
// instance ID and human-readable names.

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type pnpInfo struct {
	InstanceID   string
	Description  string
	FriendlyName string
	Location     string
	DriverKey    string
}

type pnpIndex struct {
	byDriverKey  map[string]pnpInfo
	byInstanceID map[string]pnpInfo
}

func (idx *pnpIndex) byDriver(key string) (pnpInfo, bool) {
	if idx == nil || key == "" {
		return pnpInfo{}, false
	}
	info, ok := idx.byDriverKey[strings.ToLower(key)]
	return info, ok
}

func (idx *pnpIndex) byInstance(id string) (pnpInfo, bool) {
	if idx == nil || id == "" {
		return pnpInfo{}, false
	}
	info, ok := idx.byInstanceID[strings.ToLower(id)]
	return info, ok
}

// pnpEnumerators are the only buses the hub walk needs names from. Host
// controllers live on PCI, but enumerating PCI (or all classes) costs
// 5 to 7 s on a workstation (measured), so controllers are looked up
// directly by instance ID instead; see lookupInstance.
var pnpEnumerators = []string{"USB"}

// loadPnPIndex enumerates the USB device nodes once (~80 ms).
func loadPnPIndex() (*pnpIndex, error) {
	idx := &pnpIndex{
		byDriverKey:  map[string]pnpInfo{},
		byInstanceID: map[string]pnpInfo{},
	}
	var firstErr error
	for _, enumerator := range pnpEnumerators {
		if err := idx.addEnumerator(enumerator); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if len(idx.byInstanceID) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return idx, nil
}

var (
	procCMLocateDevNodeW              = modcfgmgr32.NewProc("CM_Locate_DevNodeW")
	procCMGetDevNodeRegistryPropertyW = modcfgmgr32.NewProc("CM_Get_DevNode_Registry_PropertyW")
)

// CM_DRP_* registry property codes (cfgmgr32.h).
const (
	cmDRPDeviceDesc          = 0x01
	cmDRPDriver              = 0x0A
	cmDRPFriendlyName        = 0x0D
	cmDRPLocationInformation = 0x0E
	cmLocateDevNodeNormal    = 0
)

// lookupInstance fetches one device node by instance ID without any
// enumeration. Used for the PCI host controllers.
func lookupInstance(instanceID string) (pnpInfo, bool) {
	id16, err := windows.UTF16PtrFromString(instanceID)
	if err != nil {
		return pnpInfo{}, false
	}
	var devInst uint32
	ret, _, _ := procCMLocateDevNodeW.Call(uintptr(unsafe.Pointer(&devInst)), uintptr(unsafe.Pointer(id16)), cmLocateDevNodeNormal)
	if ret != 0 {
		return pnpInfo{}, false
	}
	info := pnpInfo{
		InstanceID:   strings.ToUpper(instanceID),
		Description:  devNodeString(devInst, cmDRPDeviceDesc),
		FriendlyName: devNodeString(devInst, cmDRPFriendlyName),
		Location:     devNodeString(devInst, cmDRPLocationInformation),
		DriverKey:    devNodeString(devInst, cmDRPDriver),
	}
	return info, info.Description != "" || info.FriendlyName != ""
}

func devNodeString(devInst uint32, prop uint32) string {
	buf := make([]uint16, 512)
	size := uint32(len(buf) * 2)
	var regType uint32
	ret, _, _ := procCMGetDevNodeRegistryPropertyW.Call(uintptr(devInst), uintptr(prop),
		uintptr(unsafe.Pointer(&regType)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
	if ret != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// byInstanceOrLookup consults the index first and falls back to a direct
// node lookup for buses the index does not cover.
func (idx *pnpIndex) byInstanceOrLookup(id string) (pnpInfo, bool) {
	if info, ok := idx.byInstance(id); ok {
		return info, true
	}
	return lookupInstance(id)
}

func (idx *pnpIndex) addEnumerator(enumerator string) error {
	set, err := windows.SetupDiGetClassDevsEx(nil, enumerator, 0,
		windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		return err
	}
	defer set.Close()

	for i := 0; ; i++ {
		data, err := set.EnumDeviceInfo(i)
		if err != nil {
			break
		}
		info := pnpInfo{
			Description:  regString(set, data, windows.SPDRP_DEVICEDESC),
			FriendlyName: regString(set, data, windows.SPDRP_FRIENDLYNAME),
			Location:     regString(set, data, windows.SPDRP_LOCATION_INFORMATION),
			DriverKey:    regString(set, data, windows.SPDRP_DRIVER),
		}
		if id, err := set.DeviceInstanceID(data); err == nil {
			// Instance IDs are case-insensitive; keep one canonical form so
			// topology ids and ETW-derived throughput ids compare equal.
			info.InstanceID = strings.ToUpper(id)
		}
		if info.DriverKey != "" {
			idx.byDriverKey[strings.ToLower(info.DriverKey)] = info
		}
		if info.InstanceID != "" {
			idx.byInstanceID[strings.ToLower(info.InstanceID)] = info
		}
	}
	return nil
}

func regString(set windows.DevInfo, data *windows.DevInfoData, prop windows.SPDRP) string {
	v, err := set.DeviceRegistryProperty(data, prop)
	if err != nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []string:
		return strings.Join(s, ";")
	}
	return ""
}

// instanceIDFromInterface derives a PnP instance ID from a device
// interface symbolic link, e.g.
//
//	\\?\USB#ROOT_HUB30#4&2c3f8e6d&0&0#{f18a0e88-...}  ->  USB\ROOT_HUB30\4&2c3f8e6d&0&0
func instanceIDFromInterface(link string) string {
	s := link
	for _, prefix := range []string{`\\?\`, `\\.\`, `\??\`} {
		s = strings.TrimPrefix(s, prefix)
	}
	if i := strings.LastIndex(s, "#{"); i >= 0 {
		s = s[:i]
	}
	return strings.ReplaceAll(s, "#", `\`)
}

// displayName picks the most useful human name from PnP data.
func (p pnpInfo) displayName() string {
	if p.FriendlyName != "" {
		return p.FriendlyName
	}
	return p.Description
}
